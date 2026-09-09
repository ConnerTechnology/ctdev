package piholelists

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeExec answers a query with the canned reply whose key prefixes the SQL,
// and records every statement passed to Exec.
type fakeExec struct {
	replies map[string]string
	err     error
	execs   []string
}

func (inst *fakeExec) Query(_ context.Context, sql string) ([]byte, error) {
	if inst.err != nil {
		return nil, inst.err
	}
	for key, reply := range inst.replies {
		if strings.HasPrefix(sql, key) {
			return []byte(reply), nil
		}
	}
	return nil, errors.New("unexpected query: " + sql)
}

func (inst *fakeExec) Exec(_ context.Context, sql string) error {
	inst.execs = append(inst.execs, sql)
	return inst.err
}

func liveExec() *fakeExec {
	return &fakeExec{replies: map[string]string{
		"SELECT d.": `[{"type":0,"domain":"a.example","enabled":1,"comment":"keep","groups":"Default"},
			{"type":3,"domain":"(\\.|^)reddit\\.com$","enabled":0,"comment":null,"groups":"Kids,Default"},
			{"type":1,"domain":"orphan.example","enabled":1,"comment":null,"groups":null}]`,
		"SELECT a.":   `[{"address":"https://example.com/ads.txt","enabled":1,"comment":"primary","groups":"Default"}]`,
		"SELECT name": `[{"name":"Default"},{"name":"Kids"}]`,
	}}
}

func TestReadLiveMapsRowsToEntries(t *testing.T) {
	got, err := ReadLive(context.Background(), liveExec())
	if err != nil {
		t.Fatalf("ReadLive: %v", err)
	}
	want := Lists{
		{Kind: Adlist, Key: "https://example.com/ads.txt", Comment: "primary", Groups: []string{"Default"}, Enabled: true},
		{Kind: Allow, Key: "a.example", Comment: "keep", Groups: []string{"Default"}, Enabled: true},
		{Kind: Deny, Key: "orphan.example", Groups: nil, Enabled: true},
		{Kind: DenyRegex, Key: `(\.|^)reddit\.com$`, Groups: []string{"Default", "Kids"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadLive mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestReadLiveHandlesEmptyOutput(t *testing.T) {
	ex := &fakeExec{replies: map[string]string{"SELECT": "\n"}}
	got, err := ReadLive(context.Background(), ex)
	if err != nil {
		t.Fatalf("ReadLive: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no entries from empty output, got %d", len(got))
	}
}

func TestReadLivePropagatesErrors(t *testing.T) {
	if _, err := ReadLive(context.Background(), &fakeExec{err: errors.New("boom")}); err == nil {
		t.Error("expected the executor error to propagate")
	}
	bad := &fakeExec{replies: map[string]string{"SELECT": "not json"}}
	if _, err := ReadLive(context.Background(), bad); err == nil {
		t.Error("expected a JSON error")
	}
}

func TestReadGroups(t *testing.T) {
	got, err := ReadGroups(context.Background(), liveExec())
	if err != nil {
		t.Fatalf("ReadGroups: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"Default", "Kids"}) {
		t.Errorf("ReadGroups = %v", got)
	}
}
