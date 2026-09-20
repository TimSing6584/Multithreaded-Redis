package protocol

import (
	"bufio"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func newReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}

// argsEqual treats nil and empty slices as equal.
func argsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// RESP arrays (what real clients send)
		{"array PING", "*1\r\n$4\r\nPING\r\n", []string{"PING"}},
		{"array SET", "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n", []string{"SET", "foo", "bar"}},
		{"array GET", "*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n", []string{"GET", "foo"}},
		{"array empty", "*0\r\n", nil},
		{"array null", "*-1\r\n", nil},
		{"array with simple string element", "*2\r\n+GET\r\n$3\r\nfoo\r\n", []string{"GET", "foo"}},
		{"array with integer element", "*3\r\n$3\r\nSET\r\n$3\r\nnum\r\n:42\r\n", []string{"SET", "num", "42"}},
		{"array with empty bulk", "*3\r\n$3\r\nSET\r\n$2\r\nk2\r\n$0\r\n\r\n", []string{"SET", "k2", ""}},
		{"array with null bulk", "*2\r\n$3\r\nGET\r\n$-1\r\n", []string{"GET", ""}},
		{"bulk containing CRLF", "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$4\r\na\r\nb\r\n", []string{"SET", "k", "a\r\nb"}},

		// Top-level non-array RESP values
		{"top-level simple string", "+SET foo bar\r\n", []string{"SET", "foo", "bar"}},
		{"top-level integer", ":0\r\n", []string{"0"}},
		{"top-level bulk", "$11\r\nSET foo bar\r\n", []string{"SET", "foo", "bar"}},
		{"top-level null bulk", "$-1\r\n", nil},

		// Inline commands
		{"inline SET", "SET foo bar\r\n", []string{"SET", "foo", "bar"}},
		{"inline LF only", "PING\n", []string{"PING"}},
		{"inline extra whitespace", "  GET   foo \r\n", []string{"GET", "foo"}},
		{"inline empty line", "\r\n", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadCommand(newReader(tc.input))
			if err != nil {
				t.Fatalf("ReadCommand(%q) unexpected error: %v", tc.input, err)
			}
			if !argsEqual(got, tc.want) {
				t.Errorf("ReadCommand(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestReadCommandErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error // nil means "any non-nil error"
	}{
		{"invalid array length", "*x\r\n", nil},
		{"invalid integer element", "*1\r\n:abc\r\n", nil},
		{"invalid bulk size", "*1\r\n$abc\r\n", nil},
		{"unknown type inside array", "*1\r\n!oops\r\n", nil},
		{"top-level invalid integer", ":abc\r\n", nil},

		// Incomplete input: the caller treats these as "buffer ran out".
		{"empty input", "", io.EOF},
		{"array missing elements", "*2\r\n$3\r\nGET\r\n", io.EOF},
		{"array header without CRLF", "*2", io.EOF},
		{"bulk payload truncated", "*1\r\n$3\r\nPI", io.ErrUnexpectedEOF},
		{"bulk missing terminator", "*1\r\n$3\r\nGET", io.ErrUnexpectedEOF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadCommand(newReader(tc.input))
			if err == nil {
				t.Fatalf("ReadCommand(%q) = %q, want error", tc.input, got)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("ReadCommand(%q) error = %v, want %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// Several commands in one buffer: each ReadCommand must consume exactly its
// own bytes so the next one starts at the right place.
func TestReadCommandPipelined(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  [][]string
	}{
		{
			name: "three arrays",
			input: "*1\r\n$4\r\nPING\r\n" +
				"*3\r\n$3\r\nSET\r\n$1\r\na\r\n$1\r\n1\r\n" +
				"*2\r\n$3\r\nGET\r\n$1\r\na\r\n",
			want: [][]string{{"PING"}, {"SET", "a", "1"}, {"GET", "a"}},
		},
		{
			name:  "top-level bulk then array",
			input: "$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n",
			want:  [][]string{{"PING"}, {"PING"}},
		},
		{
			name:  "inline then array then inline",
			input: "SET a 1\r\n*2\r\n$3\r\nGET\r\n$1\r\na\r\nPING\r\n",
			want:  [][]string{{"SET", "a", "1"}, {"GET", "a"}, {"PING"}},
		},
		{
			name:  "bulk payload containing CRLF does not desync",
			input: "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$4\r\na\r\nb\r\n*1\r\n$4\r\nPING\r\n",
			want:  [][]string{{"SET", "k", "a\r\nb"}, {"PING"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newReader(tc.input)
			for i, want := range tc.want {
				got, err := ReadCommand(r)
				if err != nil {
					t.Fatalf("command %d: unexpected error: %v", i, err)
				}
				if !argsEqual(got, want) {
					t.Errorf("command %d = %q, want %q", i, got, want)
				}
			}
			if _, err := ReadCommand(r); err != io.EOF {
				t.Errorf("after last command: err = %v, want io.EOF", err)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Value
	}{
		{"simple string", "+OK\r\n", Value{Type: '+', Str: "OK"}},
		{"error", "-ERR boom\r\n", Value{Type: '-', Str: "ERR boom"}},
		{"integer", ":42\r\n", Value{Type: ':', Num: 42}},
		{"negative integer", ":-5\r\n", Value{Type: ':', Num: -5}},
		{"bulk string", "$3\r\nfoo\r\n", Value{Type: '$', Bulk: "foo"}},
		{"empty bulk string", "$0\r\n\r\n", Value{Type: '$', Bulk: ""}},
		{"null bulk string", "$-1\r\n", Value{Type: '$', IsNull: true}},
		{"null array", "*-1\r\n", Value{Type: '*', IsNull: true}},
		{
			"flat array",
			"*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n",
			Value{Type: '*', Array: []Value{{Type: '$', Bulk: "GET"}, {Type: '$', Bulk: "foo"}}},
		},
		{
			"nested array",
			"*2\r\n*1\r\n:1\r\n$2\r\nhi\r\n",
			Value{Type: '*', Array: []Value{
				{Type: '*', Array: []Value{{Type: ':', Num: 1}}},
				{Type: '$', Bulk: "hi"},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(newReader(tc.input))
			if err != nil {
				t.Fatalf("Decode(%q) unexpected error: %v", tc.input, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Decode(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecodeLeavesReaderAtNextValue(t *testing.T) {
	r := newReader("$3\r\nfoo\r\n:7\r\n")

	first, err := Decode(r)
	if err != nil || first.Bulk != "foo" {
		t.Fatalf("first Decode = %+v, %v; want bulk \"foo\"", first, err)
	}
	second, err := Decode(r)
	if err != nil || second.Num != 7 {
		t.Fatalf("second Decode = %+v, %v; want integer 7", second, err)
	}
}
