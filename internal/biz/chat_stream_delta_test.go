package biz

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestComputeStreamSnapshotDelta(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		snapshot string
		want     string
	}{
		{
			name:     "empty current takes full snapshot",
			current:  "",
			snapshot: "hello",
			want:     "hello",
		},
		{
			name:     "snapshot extends current",
			current:  "hello",
			snapshot: "hello world",
			want:     " world",
		},
		{
			name:     "same snapshot has no delta",
			current:  "hello",
			snapshot: "hello",
			want:     "",
		},
		{
			name:     "non prefix snapshot ignored",
			current:  "hello",
			snapshot: "bye",
			want:     "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := computeStreamSnapshotDelta(tc.current, tc.snapshot)
			if got != tc.want {
				t.Fatalf("unexpected delta: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestComputeMultiContentSnapshotDelta(t *testing.T) {
	text := func(v string) schema.MessageOutputPart {
		return schema.MessageOutputPart{Type: schema.ChatMessagePartTypeText, Text: v}
	}

	current := []schema.MessageOutputPart{text("a")}

	cases := []struct {
		name     string
		snapshot []schema.MessageOutputPart
		wantLen  int
	}{
		{
			name:     "same snapshot no delta",
			snapshot: []schema.MessageOutputPart{text("a")},
			wantLen:  0,
		},
		{
			name:     "extended snapshot keeps tail",
			snapshot: []schema.MessageOutputPart{text("a"), text("b")},
			wantLen:  1,
		},
		{
			name:     "non prefix snapshot ignored",
			snapshot: []schema.MessageOutputPart{text("x"), text("b")},
			wantLen:  0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := computeMultiContentSnapshotDelta(current, tc.snapshot)
			if len(got) != tc.wantLen {
				t.Fatalf("unexpected delta len: got %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestMergeToolCallsWithChange(t *testing.T) {
	idx := 0
	existing := []schema.ToolCall{{
		Index: &idx,
		ID:    "call_1",
		Type:  "function",
		Function: schema.FunctionCall{
			Name:      "sum",
			Arguments: `{"a":1}`,
		},
	}}

	merged, changed := mergeToolCallsWithChange(existing, []schema.ToolCall{{
		Index: &idx,
		ID:    "call_1",
		Type:  "function",
		Function: schema.FunctionCall{
			Name:      "sum",
			Arguments: `{"a":1}`,
		},
	}})
	if changed {
		t.Fatalf("expected unchanged merge")
	}
	if len(merged) != 1 {
		t.Fatalf("unexpected merged len: %d", len(merged))
	}

	merged, changed = mergeToolCallsWithChange(existing, []schema.ToolCall{{
		Index: &idx,
		ID:    "call_1",
		Type:  "function",
		Function: schema.FunctionCall{
			Name:      "sum",
			Arguments: `{"a":1}...`,
		},
	}})
	if !changed {
		t.Fatalf("expected merge changed")
	}
	if merged[0].Function.Arguments != `{"a":1}...` {
		t.Fatalf("unexpected merged arguments: %q", merged[0].Function.Arguments)
	}
}
