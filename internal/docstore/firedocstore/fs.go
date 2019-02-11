// Copyright 2019 The Go Cloud Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firedocstore // import "gocloud.dev/internal/docstore/firedocstore"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"

	"cloud.google.com/go/firestore"
	vkit "cloud.google.com/go/firestore/apiv1beta1"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	pb "google.golang.org/genproto/googleapis/firestore/v1beta1"
)

type collection struct {
	client    *vkit.Client
	dbPath    string
	collPath  string
	nameField string
}

func OpenCollection(client *vkit.Client, projectID, collPath, nameField string) *docstore.Collection {
	return docstore.NewCollection(newCollection(client, projectID, collPath, nameField))
}

func newCollection(client *firestore.Client, projectID, collPath, nameField string) *collection {
	dbPath := fmt.Sprintf("projects/%s/databases/(default)", projectID)
	return &collection{
		client:    client,
		dbPath:    dbPath,
		collPath:  fmt.Sprintf("%s/documents/%s", dbPath, collPath),
		nameField: nameField,
	}
}

// func (c *collection) KeyFields() []string {
// 	return []string{c.nameField}
// }

func (c *collection) RunActions(ctx context.Context, actions []*driver.Action) (int, error) {
	groups := groupActions(actions)
	nRun := 0
	var err error
	var n int
	for _, g := range groups {
		if g[0].Kind == driver.Get {
			n, err = c.runGets(ctx, g)
		} else {
			n, err = c.runWrites(ctx, g)
		}
		nRun += n
		if err != nil {
			return nRun, err
		}
	}
	return nRun, nil
}

// Break the actions into subsequences, each of which can be one RPC.
// - Consecutive writes are grouped together.
// - Consecutive gets with the same field paths are grouped together.
// TODO: apply the constraint (from write.proto): At most one `transform` per document is allowed in a given request.
func groupActions(actions []*driver.Action) [][]*driver.Action {
	var (
		groups [][]*driver.Action
		cur    []*driver.Action
	)
	collect := func() {
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
	}

	for _, a := range actions {
		if len(cur) > 0 {
			if a.Kind != driver.Get && cur[0].Kind == driver.Get {
				collect()
			}
			if a.Kind == driver.Get && (cur[0].Kind != driver.Get || !fpsEqual(cur[0].FieldPaths, a.FieldPaths)) {
				collect()
			}
		}
		cur = append(cur, a)
	}
	collect()
	return groups
}

func (c *collection) runGets(ctx context.Context, gets []*driver.Action) (int, error) {
	req, err := c.newGetRequest(gets)
	if err != nil {
		return err
	}
	streamClient, err := c.client.BatchGetDocuments(withResourceHeader(ctx, req.Database), req)
	if err != nil {
		return err
	}
	// Read the stream and organize by path, since results may arrive out of order.
	resps := map[string]*pb.BatchGetDocumentsResponse{}
	for {
		resp, err := streamClient.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		var path string
		switch r := resp.Result.(type) {
		case *pb.BatchGetDocumentsResponse_Found:
			resps[r.Found.Name] = resp
		case *pb.BatchGetDocumentsResponse_Missing:
			resps[r.Missing] == nil
		default:
			return 0, driver.Errorf(Invalid, nil, "unknown BatchGetDocumentsResponse result type")
		}
	}
	for i, path := range req.Documents {
		resp, ok := resps[path]
		if !ok {
			return i, driver.Errorf(Invalid, nil, "no BatchGetDocumentsResponse for %q", path)
		}
		if resp == nil {
			return i, driver.Errorf(NotFound, nil, "document at path %q is missing", path)
		}
		pdoc := resp.(*pb.BatchGetDocumentsResponse_Found).Found
		if err := copyFields(gets[i].Doc, pdoc, gets[i].FieldPaths); err != nil {
			return i, err
		}
		_ = gets[i].Doc.SetField(docstore.RevisionField, pdoc.UpdateTime)
	}
	return len(gets), nil
}

func (c *collection) newGetRequest(gets []*driver.Action) (*pb.BatchGetDocumentsRequest, error) {
	req := &pb.BatchGetDocumentsRequest{Database: c.dbPath}
	for _, a := range gets {
		docName, err := c.docName(a.Doc)
		if err != nil {
			return nil, err
		}
		req.Documents = append(rec.Documents, c.collPath+"/"+docName)
	}
	// groupActions has already made sure that all the actions have the same field paths,
	// so just use the first one.
	fps := gets[0].FieldPaths
	if fps != nil {
		req.Mask = &pb.DocumentMask{FieldPaths: make([]string, len(fps))}
		for i, fp := range fps {
			req.Mask.FieldPaths[i] = strings.Join(fp, ".")
		}
	}
	return req, nil
}

func copyFields(ddoc driver.Document, pdoc *pb.Document, fps [][]string) error {
	if fps != nil {
		return errors.New("unimp")
	}
	return decodeDoc
}

func (c *collection) runWrites(ctx context.Context, actions []*driver.Action) error {
	var pws []*pb.Write
	newNames := make([]string, len(actions)) // from Creates without a name
	for i, a := range actions {
		ws, nn, err := actionToWrites(a)
		if err != nil {
			return err
		}
		newNames[i] = nn
		pws = append(pws, ws...)
	}
	wrs, err := c.commit(ctx, pws)
	if err != nil {
		return err
	}
	// Now that we've successfully done the action, set the names for newly created docs
	// that weren't given a name by the caller.
	for i, nn := range newNames {
		if nn != "" {
			_ = actions[i].Doc.SetField(c.nameField, nn)
		}
	}
	// Set the revision fields of all docs to the returned update times.
	for i, wr := range wrs {
		// Ignore errors. It's fine if the doc doesn't have a revision field.
		// (We also could get an error if that field is unsettable for some reason, but
		// we just decide to ignore those as well.)
		_ = actions[i].doc.SetField(docstore.RevisionField, wr.UpdateTime)
	}
	return nil
}

func (c *collection) actionToWrites(a *driver.Action) ([]*pb.Write, string, error) {
	docName, err := c.docName(a.Doc)
	if err != nil {
		return nil, "", err
	}
	var (
		w       *pb.Write
		ws      []*pb.Write
		newName string
		err     error
	)
	switch a.Kind {
	case driver.Create:
		// Make a name for this document if it doesn't have one.
		if docName == "" {
			docName = driver.UniqueString()
			newName = docName
		}
		w, err = c.putWrite(a.Doc, docName, &pb.Precondition{ConditionType: &pb.Precondition_Exists{false}})

	case driver.Replace:
		pc, err := revisionPrecondition(a.Doc)
		if err != nil {
			return nil, "", err
		}
		if pc == nil {
			pc = &pb.Precondition_Exists{true}
		}
		w, err = c.putWrite(a.Doc, docName, &pb.Precondition{ConditionType: pc})

	case driver.Put:
		pc, err := revisionPrecondition(a.Doc)
		if err != nil {
			return i, err
		}
		w, err = c.putWrite(a.Doc, docName, pc)

	case driver.Update:
		ws, err = c.updateWrites(a.Doc, a.Mods)

	case driver.Delete:
		w, err = c.deleteWrite(a.Doc, docName)

	default:
		err = driver.Errorf(driver.Internal, "bad action %+v", a)
	}
	if err != nil {
		return nil, "", err
	}
	if ws == nil {
		ws = []*pb.Write{w}
	}
	return ws, newName, nil
}

func (c *collection) putWrite(doc driver.Document, docName string, pc *pb.Precondition) (*pb.Write, error) {
	pdoc, err := encodeDoc(doc)
	if err != nil {
		return nil, err
	}
	pdoc.Name = c.collPath + "/" + docName
	return &pb.Write{
		Operation:       &pb.Write_Update{pdoc},
		CurrentDocument: pc,
	}, nil
}

func (c *collection) deleteWrite(doc driver.Document, docName string) (*pb.Write, error) {
	pc, err := revisionPrecondition(doc)
	if err != nil {
		return nil, err
	}
	return &pb.Write{
		Operation:       &pb.Write_Delete{c.collPath + "/" + docName},
		CurrentDocument: pc,
	}, nil
}

// We may need two writes: one for setting and deleting values, the other for transforms.
func (c *collection) updateWrites(doc driver.Document, docName string, mods []driver.Mod) ([]*pb.Write, error) {
	pc, err := revisionPrecondition(doc)
	if err != nil {
		return nil, err
	}
	pdoc := &pb.Document{Name: c.collPath + "/" + docName}
	var fps []string
	for _, m := range mods {
		// TODO: encode field path components
		fps = append(fps, toServiceFieldPath(m.FieldPath))
		// If m.Value is nil, we want to delete it. In that case, put the field
		// in the mask but not in the doc.
		if m.Value != nil {
			pv, err := encodeValue(m.Value)
			if err != nil {
				return nil, err
			}
			if err := setFieldPath(pdoc, m.FieldPath, pv); err != nil {
				return nil, err
			}
		}
	}
	w := &pb.Write{
		Operation:       &pb.Write_Update{pdoc},
		UpdateMask:      &pb.DocumentMask{FieldPaths: fps},
		CurrentDocument: pc,
	}
	// For now, we don't have any transforms.
	return []*pb.Write{w}, nil
}

////////////////
// From fieldpath.go in cloud.google.com/go/firestore.

func toServiceFieldPath([]string) string {
	cs := make([]string, len(fp))
	for i, c := range fp {
		cs[i] = toServiceFieldPathComponent(c)
	}
	return strings.Join(cs, ".")
}

// Google SQL syntax for an unquoted field.
var unquotedFieldRegexp = regexp.MustCompile("^[A-Za-z_][A-Za-z_0-9]*$")

// toServiceFieldPathComponent returns a string that represents key and is a valid
// Firestore field path component.
func toServiceFieldPathComponent(key string) string {
	if unquotedFieldRegexp.MatchString(key) {
		return key
	}
	var buf bytes.Buffer
	buf.WriteRune('`')
	for _, r := range key {
		if r == '`' || r == '\\' {
			buf.WriteRune('\\')
		}
		buf.WriteRune(r)
	}
	buf.WriteRune('`')
	return buf.String()
}

////////////////

func revisionPrecondition(doc driver.Document) (*pb.Precondition, error) {
	v, err := doc.GetField(docstore.RevisionField)
	if err != nil { // revision field not present
		return nil, nil
	}
	rev, ok := v.(*tspb.Timestamp)
	if !ok {
		return nil, driver.Errorf(driver.InvalidArgument,
			"%s field contains wrong type: got %T, want proto Timestamp",
			docstore.RevisionField, v)
	}
	if rev == nil || (rev.Seconds == 0 && rev.Nanos == 0) { // ignore a missing or zero revision
		return nil, nil
	}
	return &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{rev}}, nil
}

// 		// Make a name for this document if it doesn't have one.
// 		if docName == "" {
// 			docName = driver.UniqueString()
// 		}

// Firestore Commit constraint:
// At most one `transform` per document is allowed in a given request.
// An `update` cannot follow a `transform` on the same document in a given
// request.

func (c *collection) commit(ctx context.Context, ws []*pb.Write) ([]*pb.WriteResult, error) {
	req := &pb.CommitRequest{
		Database: c.dbPath,
		Writes:   ws,
	}
	res, err := c.client.Commit(withResourceHeader(ctx, req.Database), req)
	if err != nil {
		return nil, err
	}
	if len(res.WriteResults) != len(ws) {
		return nil, driver.Errorf(driver.Internal, nil, "wrong number of WriteResults from firestore commit")
	}
	return res.WriteResults, nil
}

func (c *collection) docName(doc driver.Document) (string, error) {
	n, err := doc.GetField(c.nameField)
	if err != nil {
		// Return missing field as empty string.
		return "", nil
	}
	vn := reflect.ValueOf(n)
	if vn.Kind() != reflect.String {
		return "", fmt.Errorf("key field %q with value %v is not a string", c.nameField, n)
	}
	return vn.String(), nil
}

func (c *collection) RunQuery(ctx context.Context, q *driver.Query) error {
	return errors.New("unimp")
}

func fpsEqual(fps1, fps2 [][]string) bool {
	// TODO?: We really care about sets of field paths, but that's too tedious to determine.
	if len(fps1) != len(fps2) {
		return false
	}
	for i, fp1 := range fps1 {
		if !fpEqual(fp1, fps2[i]) {
			return false
		}
	}
	return true
}

func fpEqual(fp1, fp2 []string) bool {
	if len(fp1) != len(fp2) {
		return false
	}
	for i, s1 := range fp1 {
		if s1 != fp2[i] {
			return false
		}
	}
	return true
}
