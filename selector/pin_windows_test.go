//go:build windows

package selector

import "testing"

func TestDrawPinWithFallback(t *testing.T) {
	for _, test := range []struct {
		name    string
		results []int32
		wantErr bool
	}{
		{"success", []int32{20}, false},
		{"zero scan lines retries", []int32{0, 20}, false},
		{"GDI error retries", []int32{-1, 20}, false},
		{"both attempts draw nothing", []int32{0, 0}, true},
		{"both attempts fail", []int32{-1, -1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := drawPinWithFallback(func(mode uintptr) int32 {
				if calls >= len(test.results) {
					t.Fatal("unexpected extra drawing attempt")
				}
				wantMode := uintptr(dibStretchHalftone)
				if calls > 0 {
					wantMode = 3
				}
				if mode != wantMode {
					t.Fatalf("stretch mode = %d, want %d", mode, wantMode)
				}
				result := test.results[calls]
				calls++
				return result
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if calls != len(test.results) {
				t.Fatalf("drawing attempts = %d, want %d", calls, len(test.results))
			}
		})
	}
}
