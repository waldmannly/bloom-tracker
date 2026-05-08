//go:build ignore
// +build ignore

// Run with: go run generate_icons.go
// Generates PWA icon PNGs for Bloom Period Tracker

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

var (
	bgColor    = color.NRGBA{0xFF, 0xF9, 0xF9, 0xFF} // #FFF9F9
	petalColor = color.NRGBA{0xD4, 0x78, 0x8C, 0xFF} // #D4788C (rose)
	centerColor = color.NRGBA{0xF2, 0xD1, 0xD9, 0xFF} // #F2D1D9 (rose-light)
	petalDark  = color.NRGBA{0xA8, 0x55, 0x66, 0xFF} // #A85566 (rose-dark)
)

func main() {
	dir := filepath.Join("static", "icons")
	os.MkdirAll(dir, 0755)

	// Standard icon sizes
	sizes := []int{16, 32, 72, 96, 128, 144, 152, 167, 180, 192, 384, 512}
	for _, size := range sizes {
		img := generateIcon(size, false)
		savePNG(filepath.Join(dir, fmt.Sprintf("icon-%d.png", size)), img)
		fmt.Printf("Generated icon-%d.png\n", size)
	}

	// Maskable icons (with extra padding for safe zone)
	maskableSizes := []int{192, 512}
	for _, size := range maskableSizes {
		img := generateIcon(size, true)
		savePNG(filepath.Join(dir, fmt.Sprintf("maskable-%d.png", size)), img)
		fmt.Printf("Generated maskable-%d.png\n", size)
	}

	// Splash screens
	splashes := [][2]int{
		{640, 1136},
		{750, 1334},
		{1125, 2436},
		{1170, 2532},
		{1242, 2208},
		{1284, 2778},
	}
	for _, s := range splashes {
		img := generateSplash(s[0], s[1])
		savePNG(filepath.Join(dir, fmt.Sprintf("splash-%dx%d.png", s[0], s[1])), img)
		fmt.Printf("Generated splash-%dx%d.png\n", s[0], s[1])
	}

	fmt.Println("All icons generated!")
}

func generateIcon(size int, maskable bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// Fill background
	bg := bgColor
	if maskable {
		// Maskable icons need solid bg color filling the entire canvas
		bg = color.NRGBA{0xD4, 0x78, 0x8C, 0xFF}
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Draw the cherry blossom flower
	cx := float64(size) / 2
	cy := float64(size) / 2

	var flowerRadius float64
	if maskable {
		// Maskable: keep flower in safe zone (inner 80%)
		flowerRadius = float64(size) * 0.3
	} else {
		flowerRadius = float64(size) * 0.38
	}

	// Draw 5 petals
	for i := 0; i < 5; i++ {
		angle := float64(i) * (2 * math.Pi / 5) - math.Pi/2
		px := cx + flowerRadius*0.55*math.Cos(angle)
		py := cy + flowerRadius*0.55*math.Sin(angle)
		petalR := flowerRadius * 0.5
		drawFilledCircle(img, px, py, petalR, petalColor)
		// Inner highlight on each petal
		drawFilledCircle(img, px-petalR*0.15, py-petalR*0.15, petalR*0.4, centerColor)
	}

	// Draw center
	centerR := flowerRadius * 0.28
	drawFilledCircle(img, cx, cy, centerR, centerColor)
	// Center dot
	drawFilledCircle(img, cx, cy, centerR*0.4, petalDark)

	return img
}

func generateSplash(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw the flower centered, sized appropriately
	iconSize := width / 3
	icon := generateIcon(iconSize, false)
	offsetX := (width - iconSize) / 2
	offsetY := (height - iconSize) / 2 - height/10 // slightly above center
	draw.Draw(img, image.Rect(offsetX, offsetY, offsetX+iconSize, offsetY+iconSize), icon, image.Point{}, draw.Over)

	return img
}

func drawFilledCircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	minX := int(math.Floor(cx - r))
	maxX := int(math.Ceil(cx + r))
	minY := int(math.Floor(cy - r))
	maxY := int(math.Ceil(cy + r))

	bounds := img.Bounds()
	if minX < bounds.Min.X { minX = bounds.Min.X }
	if minY < bounds.Min.Y { minY = bounds.Min.Y }
	if maxX > bounds.Max.X { maxX = bounds.Max.X }
	if maxY > bounds.Max.Y { maxY = bounds.Max.Y }

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= r {
				// Anti-aliasing at the edge
				if dist > r-1.0 {
					alpha := r - dist
					if alpha < 0 { alpha = 0 }
					existing := img.NRGBAAt(x, y)
					blended := blendColors(existing, c, alpha)
					img.SetNRGBA(x, y, blended)
				} else {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
}

func blendColors(bg, fg color.NRGBA, alpha float64) color.NRGBA {
	a := alpha
	ia := 1.0 - a
	return color.NRGBA{
		R: uint8(float64(fg.R)*a + float64(bg.R)*ia),
		G: uint8(float64(fg.G)*a + float64(bg.G)*ia),
		B: uint8(float64(fg.B)*a + float64(bg.B)*ia),
		A: 0xFF,
	}
}

func savePNG(path string, img *image.NRGBA) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding %s: %v\n", path, err)
		os.Exit(1)
	}
}
