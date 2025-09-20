package windows

import (
	"fmt"
	"image/color"
	"regexp"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// Wrapping cell components in a struct for easy access
type FileCell struct {
	Container  *fyne.Container
	Background *canvas.Rectangle
	Btn        *widget.Button
	Img        *canvas.Image
	Files      []fyne.URI
	Regex      *regexp.Regexp
	FilesMu    sync.RWMutex

	OnLoaded func(count int) // Will be the max frames for the slider
}

// Struct helper function to return the length of the slice of file paths
func (fc *FileCell) Len() int {
	fc.FilesMu.RLock()
	defer fc.FilesMu.RUnlock()

	return len(fc.Files)
}

// Struct helper function to create a canvas image object from a specific index in the slice of file paths
func (fc *FileCell) Show(i int) error {
	fc.FilesMu.RLock()
	defer fc.FilesMu.RUnlock()

	if i < 0 || i >= len(fc.Files) {
		return fmt.Errorf("index for image file out of range: %d (LENGTH: %d)", i, len(fc.Files))
	}

	// Load URI Path
	u := fc.Files[i]

	// Decode via cache and reuse a single canvas.Image instance
	img, err := DecodeURI(u)
	if err != nil {
		return err
	}

	if fc.Img == nil {
		fc.Img = canvas.NewImageFromImage(img)
		fc.Img.FillMode = canvas.ImageFillContain
		fc.Background.FillColor = color.Black
		fc.Container.Objects = []fyne.CanvasObject{fc.Background, fc.Img}
	} else {
		// Reuse existing image widget, switch to in-memory image
		fc.Img.Image = img
		fc.Img.Resource = nil
	}

	fc.Container.Refresh()
	return nil
}

// Struct helper function to create binding to show the image in the slice of files equal to the bind value
func (fc *FileCell) BindToIndex(idx binding.Float) {
	idx.AddListener(binding.NewDataListener(func() {
		v, _ := idx.Get()

		// clamp into range
		n := fc.Len()
		if n == 0 {
			return
		}
		i := int(v)
		if i < 0 {
			i = 0
		}
		// prevent index out of range errors
		if i >= n {
			i = n - 1
		}

		fyne.Do(func() { _ = fc.Show(i) })
	}))
}
