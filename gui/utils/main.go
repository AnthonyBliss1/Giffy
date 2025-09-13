package utils

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type ResizeOptions struct {
	InputDir  string
	OutputDir string
	MaxW      int
	MaxH      int
}

func ProcessDir(opts ResizeOptions) (processed, resized, errors int) {
	err := filepath.WalkDir(opts.InputDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			errors++
			log.Printf("Skip %s: %v", path, walkErr)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".png" {
			return nil
		}
		rel, err := filepath.Rel(opts.InputDir, path)
		if err != nil {
			errors++
			log.Printf("Rel error %s: %v", path, err)
			return nil
		}
		outPath := filepath.Join(opts.OutputDir, rel)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			errors++
			log.Printf("Mkdir error %s: %v", outPath, err)
			return nil
		}

		changed, err := processFile(path, outPath, opts)
		processed++
		if changed {
			resized++
		}
		if err != nil {
			errors++
			log.Printf("Error processing %s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		errors++
		log.Printf("Walk error: %v", err)
	}
	return
}

func processFile(inPath, outPath string, opts ResizeOptions) (resized bool, err error) {
	// Read image dimensions quickly
	f, err := os.Open(inPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return false, fmt.Errorf("decode config: %w", err)
	}
	srcW, srcH := cfg.Width, cfg.Height
	dstW, dstH := computeTargetSize(srcW, srcH, opts.MaxW, opts.MaxH)

	// If no size change, either copy or re-encode (copy by default with -resizeonly)
	if dstW == srcW && dstH == srcH {
		if err := copyFile(inPath, outPath); err != nil {
			return false, fmt.Errorf("copy no-change: %w", err)
		}
		log.Printf("Copied (no resize needed): %s", relFrom(outPath, opts.OutputDir))
		return false, nil

		// Fallthrough to re-encode without resizing
	}

	// Reopen to decode full image
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		f2, e2 := os.Open(inPath)
		if e2 != nil {
			return false, e2
		}
		f = f2
	}
	img, err := png.Decode(f)
	if err != nil {
		return false, fmt.Errorf("decode: %w", err)
	}

	var outImg image.Image
	if dstW == srcW && dstH == srcH {
		outImg = img // no resize, but re-encode
	} else {
		outNRGBA := resizeBox(toNRGBA(img), dstW, dstH)
		outImg = outNRGBA
		resized = true
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return resized, fmt.Errorf("create out: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// No compression tuning: use default encoder
	if err := png.Encode(outFile, outImg); err != nil {
		return resized, fmt.Errorf("encode: %w", err)
	}

	action := "Re-encoded (no resize)"
	if resized {
		action = fmt.Sprintf("Resized %dx%d -> %dx%d", srcW, srcH, dstW, dstH)
	}
	log.Printf("%s: %s", action, relFrom(outPath, opts.OutputDir))
	return resized, nil
}

func computeTargetSize(srcW, srcH, maxW, maxH int) (int, int) {
	if maxW <= 0 && maxH <= 0 {
		return srcW, srcH
	}
	if maxW <= 0 {
		maxW = int(math.Ceil(float64(srcW) * float64(maxH) / float64(srcH)))
	}
	if maxH <= 0 {
		maxH = int(math.Ceil(float64(srcH) * float64(maxW) / float64(srcW)))
	}
	rw := float64(maxW) / float64(srcW)
	rh := float64(maxH) / float64(srcH)
	scale := math.Min(rw, rh)
	if scale >= 1.0 {
		// Do not upscale
		return srcW, srcH
	}
	dstW := int(math.Round(float64(srcW) * scale))
	dstH := int(math.Round(float64(srcH) * scale))
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	return dstW, dstH
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

// resizeBox performs area-averaging downscale and simple bilinear when upscaling (though we never upscale here).
func resizeBox(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	if dstW == src.Bounds().Dx() && dstH == src.Bounds().Dy() {
		out := image.NewNRGBA(src.Bounds())
		copy(out.Pix, src.Pix)
		return out
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))

	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)

	// Downscaling with box filter
	for y := 0; y < dstH; y++ {
		srcY0 := int(math.Floor(float64(y) * scaleY))
		srcY1 := int(math.Floor(float64(y+1) * scaleY))
		if srcY1 <= srcY0 {
			srcY1 = srcY0 + 1
		}
		if srcY1 > srcH {
			srcY1 = srcH
		}
		for x := 0; x < dstW; x++ {
			srcX0 := int(math.Floor(float64(x) * scaleX))
			srcX1 := int(math.Floor(float64(x+1) * scaleX))
			if srcX1 <= srcX0 {
				srcX1 = srcX0 + 1
			}
			if srcX1 > srcW {
				srcX1 = srcW
			}
			var sumR, sumG, sumB, sumA uint64
			count := uint64((srcX1 - srcX0) * (srcY1 - srcY0))
			for sy := srcY0; sy < srcY1; sy++ {
				for sx := srcX0; sx < srcX1; sx++ {
					c := srcAtNRGBA(src, sx, sy)
					sumR += uint64(c.R)
					sumG += uint64(c.G)
					sumB += uint64(c.B)
					sumA += uint64(c.A)
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(sumR / count)
			dst.Pix[i+1] = uint8(sumG / count)
			dst.Pix[i+2] = uint8(sumB / count)
			dst.Pix[i+3] = uint8(sumA / count)
		}
	}
	return dst
}

func srcAtNRGBA(img *image.NRGBA, x, y int) (c struct{ R, G, B, A uint8 }) {
	i := img.PixOffset(x, y)
	c.R = img.Pix[i+0]
	c.G = img.Pix[i+1]
	c.B = img.Pix[i+2]
	c.A = img.Pix[i+3]
	return
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func relFrom(path, root string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}
