package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
)

const (
	logoColumns = 20
	logoRows    = 10
)

//go:embed assets/logo.png
var logoPNG []byte

var appLogoBanner = buildLogoBanner()

func buildLogoBanner() string {
	src, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return ""
	}

	resized := resizeNearest(src, logoColumns, logoRows*2)
	var out strings.Builder
	for y := 0; y < logoRows; y++ {
		for x := 0; x < logoColumns; x++ {
			topR, topG, topB := rgbaToWhite(resized.At(x, y*2))
			bottomR, bottomG, bottomB := rgbaToWhite(resized.At(x, y*2+1))
			out.WriteString(ansiCell(topR, topG, topB, bottomR, bottomG, bottomB))
		}
		if y < logoRows-1 {
			out.WriteByte('\n')
		}
	}
	out.WriteString("\x1b[0m")
	return out.String()
}

func resizeNearest(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	if width <= 0 || height <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	for y := 0; y < height; y++ {
		srcY := bounds.Min.Y + (y*srcHeight)/height
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + (x*srcWidth)/width
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func rgbaToWhite(c color.Color) (uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()
	srcR := uint8(r >> 8)
	srcG := uint8(g >> 8)
	srcB := uint8(b >> 8)
	alpha := uint8(a >> 8)
	if alpha == 255 {
		return srcR, srcG, srcB
	}

	return blendOnWhite(srcR, alpha), blendOnWhite(srcG, alpha), blendOnWhite(srcB, alpha)
}

func blendOnWhite(src uint8, alpha uint8) uint8 {
	return uint8((int(src)*int(alpha) + 255*(255-int(alpha))) / 255)
}

func ansiCell(topR, topG, topB, bottomR, bottomG, bottomB uint8) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm\u2580", topR, topG, topB, bottomR, bottomG, bottomB)
}
