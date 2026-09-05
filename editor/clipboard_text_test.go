package editor

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrapClipboardText(t *testing.T) {
	measure := func(text string) (int, error) { return utf8.RuneCountInString(text), nil }
	tests := []struct {
		text  string
		width int
		want  []string
	}{
		{"中文测试ABC", 3, []string{"中文测", "试AB", "C"}},
		{"hello world", 7, []string{"hello ", "world"}},
		{"a\r\n\r\nb\rc", 10, []string{"a", "", "b", "c"}},
		{"a\tb", 10, []string{"a    b"}},
		{"1234567890", 3, []string{"123", "456", "789", "0"}},
		{"one\n", 10, []string{"one", ""}},
	}
	for _, test := range tests {
		got, err := wrapClipboardText(test.text, test.width, 100, measure)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: got %q / %v, want %q", test.text, got, err, test.want)
		}
	}
	if _, err := wrapClipboardText("abc", 1, 2, measure); err == nil {
		t.Fatal("silently truncated oversized text")
	}
	if _, err := wrapClipboardText("abc", 10, 10, func(string) (int, error) { return 0, errors.New("font failure") }); err == nil {
		t.Fatal("measurement failure ignored")
	}
}

func TestWrapClipboardTextLongParagraphPreservesCharacters(t *testing.T) {
	text := strings.Repeat("中文word ", 1000)
	got, err := wrapClipboardText(text, 20, 10000, func(s string) (int, error) { return utf8.RuneCountInString(s), nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "") != text {
		t.Fatal("characters were lost")
	}
	for _, line := range got {
		if utf8.RuneCountInString(line) > 20 {
			t.Fatal("line exceeds width")
		}
	}
}
