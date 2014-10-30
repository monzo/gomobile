// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin
// TODO(crawshaw): Run tests on other OSs when more contexts are supported.

package glutil

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"os"
	"testing"

	"code.google.com/p/go.mobile/geom"
	"code.google.com/p/go.mobile/gl"
)

func TestImage(t *testing.T) {
	// GL testing strategy:
	// 	1. Create an offscreen framebuffer object.
	// 	2. Configure framebuffer to render to a GL texture.
	//	3. Run test code: use glimage to draw testdata.
	//	4. Copy GL texture back into system memory.
	//	5. Compare to a pre-computed image.

	f, err := os.Open("../../testdata/testpattern.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	ctxGL := createContext()
	defer ctxGL.destroy()

	const (
		pixW = 100
		pixH = 100
		ptW  = geom.Pt(50)
		ptH  = geom.Pt(50)
	)
	geom.PixelsPerPt = float32(pixW) / float32(ptW)
	geom.Width = ptW
	geom.Height = ptH

	fb := gl.GenFramebuffer()
	gl.BindFramebuffer(gl.FRAMEBUFFER, fb)

	colorBuf := gl.GenRenderbuffer()
	gl.BindRenderbuffer(gl.RENDERBUFFER, colorBuf)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA, pixW, pixH)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, colorBuf)

	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		t.Fatalf("framebuffer create failed: %v", status)
	}

	gl.ClearColor(0, 0, 1, 1) // blue
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Viewport(0, 0, pixW, pixH)

	m := NewImage(src.Bounds().Dx(), src.Bounds().Dy())
	b := m.RGBA.Bounds()
	draw.Draw(m.RGBA, b, src, src.Bounds().Min, draw.Src)
	m.Upload()
	b.Min.X += 10
	b.Max.Y /= 2

	ptTopLeft := geom.Point{3, 15}
	ptBottomRight := geom.Point{48, 46}
	m.Draw(geom.Rectangle{ptTopLeft, ptBottomRight}, b)

	// For unknown reasons, a windowless OpenGL context on darwin
	// renders upside down. That is, a quad covering the initial
	// viewport spans:
	//
	//	(-1, -1) ( 1, -1)
	//	(-1,  1) ( 1,  1)
	//
	// To avoid modifying live code for tests, we flip the rows
	// recovered from the renderbuffer. We are not the first:
	//
	// http://lists.apple.com/archives/mac-opengl/2010/Jun/msg00080.html
	got := image.NewRGBA(image.Rect(0, 0, pixW, pixH))
	upsideDownPix := make([]byte, len(got.Pix))
	gl.ReadPixels(upsideDownPix, 0, 0, pixW, pixH, gl.RGBA, gl.UNSIGNED_BYTE)
	for y := 0; y < pixH; y++ {
		i0 := (pixH - 1 - y) * got.Stride
		i1 := i0 + pixW*4
		copy(got.Pix[y*got.Stride:], upsideDownPix[i0:i1])
	}

	drawCross(got, 0, 0)
	drawCross(got, int(ptTopLeft.X.Px()), int(ptTopLeft.Y.Px()))
	drawCross(got, int(ptBottomRight.X.Px()), int(ptBottomRight.Y.Px()))
	drawCross(got, pixW-1, pixH-1)

	const wantPath = "../../testdata/testpattern-window.png"
	f, err = os.Open(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	wantSrc, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	want, ok := wantSrc.(*image.RGBA)
	if !ok {
		b := wantSrc.Bounds()
		want = image.NewRGBA(b)
		draw.Draw(want, b, wantSrc, b.Min, draw.Src)
	}

	if !imageEq(got, want) {
		// Write out the image we got.
		f, err = ioutil.TempFile("", "testpattern-window-got")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		gotPath := f.Name() + ".png"
		f, err = os.Create(gotPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, got); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Errorf("got\n%s\nwant\n%s", gotPath, wantPath)
	}
}

func drawCross(m *image.RGBA, x, y int) {
	c := color.RGBA{0xff, 0, 0, 0xff} // red
	m.SetRGBA(x+0, y-2, c)
	m.SetRGBA(x+0, y-1, c)
	m.SetRGBA(x-2, y+0, c)
	m.SetRGBA(x-1, y+0, c)
	m.SetRGBA(x+0, y+0, c)
	m.SetRGBA(x+1, y+0, c)
	m.SetRGBA(x+2, y+0, c)
	m.SetRGBA(x+0, y+1, c)
	m.SetRGBA(x+0, y+2, c)
}

func eqEpsilon(x, y uint8) bool {
	const epsilon = 5
	return x-y < epsilon || y-x < epsilon
}

func colorEq(c0, c1 color.RGBA) bool {
	return eqEpsilon(c0.R, c1.R) && eqEpsilon(c0.G, c1.G) && eqEpsilon(c0.B, c1.B) && eqEpsilon(c0.A, c1.A)
}

func imageEq(m0, m1 *image.RGBA) bool {
	b0 := m0.Bounds()
	b1 := m1.Bounds()
	if b0 != b1 {
		return false
	}
	for y := b0.Min.Y; y < b0.Max.Y; y++ {
		for x := b0.Min.X; x < b0.Max.X; x++ {
			c0, c1 := m0.At(x, y).(color.RGBA), m1.At(x, y).(color.RGBA)
			if !colorEq(c0, c1) {
				return false
			}
		}
	}
	return true
}
