package screenshotwin

import (
	"image"
	"image/color"
	"testing"
)

func TestBidirectionalStitcherExtendsUpAndDown(t *testing.T) {
	source := createTestImage(320, 1800)
	stitcher, err := NewBidirectionalStitcher(crop(source, 600, 600), DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		position    int
		wantDelta   int
		wantPageTop int
		wantTop     int
		wantBottom  int
	}{
		{position: 300, wantDelta: -300, wantPageTop: -300, wantTop: 300},
		{position: 0, wantDelta: -300, wantPageTop: -600, wantTop: 300},
		{position: 300, wantDelta: 300, wantPageTop: -300},
		{position: 600, wantDelta: 300, wantPageTop: 0},
		{position: 900, wantDelta: 300, wantPageTop: 300, wantBottom: 300},
		{position: 1200, wantDelta: 300, wantPageTop: 600, wantBottom: 300},
	} {
		result, addErr := stitcher.Add(crop(source, test.position, 600))
		if addErr != nil {
			t.Fatalf("position %d: %v", test.position, addErr)
		}
		if !result.Matched || result.Delta != test.wantDelta || result.Position != test.wantPageTop || result.AddedTop != test.wantTop || result.AddedBottom != test.wantBottom {
			t.Fatalf("position %d: result = %+v", test.position, result)
		}
	}
	assertImagesEqual(t, stitcher.Finish(), source)
}

func TestBidirectionalStitcherRelocalizesToCapturedPosition(t *testing.T) {
	source := createTestImage(320, 1800)
	stitcher, err := NewBidirectionalStitcher(crop(source, 0, 600), DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, position := range []int{300, 600, 900} {
		result, addErr := stitcher.Add(crop(source, position, 600))
		if addErr != nil || !result.Matched {
			t.Fatalf("position %d: result=%+v err=%v", position, result, addErr)
		}
	}
	result, err := stitcher.Add(crop(source, 0, 600))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || !result.Relocalized || result.Position != 0 || result.Delta != -900 || result.AddedTop != 0 || result.AddedBottom != 0 {
		t.Fatalf("relocalized result = %+v", result)
	}
}

func TestBidirectionalStitcherPreservesFirstPixelsOnRevisit(t *testing.T) {
	source := createTestImage(320, 1200)
	first := crop(source, 0, 600)
	stitcher, err := NewBidirectionalStitcher(first, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result, addErr := stitcher.Add(crop(source, 300, 600)); addErr != nil || !result.Matched {
		t.Fatalf("initial move: result=%+v err=%v", result, addErr)
	}
	revisit := crop(source, 0, 600)
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			revisit.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	result, err := stitcher.Add(revisit)
	if err != nil || !result.Matched || result.AddedTop != 0 || result.AddedBottom != 0 {
		t.Fatalf("revisit: result=%+v err=%v", result, err)
	}
	assertImagesEqual(t, stitcher.Image(), crop(source, 0, 900))
}

func TestBidirectionalStitcherRejectsUnseenJumpWithoutOverlap(t *testing.T) {
	source := createTestImage(320, 1800)
	stitcher, err := NewBidirectionalStitcher(crop(source, 0, 600), DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	result, err := stitcher.Add(crop(source, 700, 600))
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched {
		t.Fatalf("unseen jump unexpectedly matched: %+v", result)
	}
	assertImagesEqual(t, stitcher.Image(), crop(source, 0, 600))
}

func TestBidirectionalStitcherRejectsAfterFinish(t *testing.T) {
	frame := createTestImage(40, 40)
	stitcher, err := NewBidirectionalStitcher(frame, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	stitcher.Finish()
	if _, err := stitcher.Add(frame); err == nil {
		t.Fatal("Add after Finish succeeded")
	}
}

func TestBidirectionalStitcherDropsOverlyCommonAnchors(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 320, 600))
	stitcher, err := NewBidirectionalStitcher(frame, DefaultMatchOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, ambiguous := stitcher.ambiguous[0]; !ambiguous {
		t.Fatal("flat-page anchor was not marked ambiguous")
	}
	if positions := stitcher.anchors[0]; len(positions) != 0 {
		t.Fatalf("ambiguous anchor retained %d positions", len(positions))
	}
}
