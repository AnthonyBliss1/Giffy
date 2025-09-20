package windows

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/anthonybliss1/giffy/gui/resize"
	ui "github.com/anthonybliss1/giffy/theme"
	"github.com/ncruces/zenity"
	xdraw "golang.org/x/image/draw"
)

// Helper function to create each cell in the grid layout
func NewFileCell(a fyne.App, parent fyne.Window) *FileCell {
	// Initialize collection struct
	fc := &FileCell{}

	// Set default frame regex to reTrailingDigits
	fc.Regex = reTrailingDigits

	// Background rectangle that will contain the gif
	fc.Background = canvas.NewRectangle(ui.Giffy)

	// File upload button inside the cell (using zenity for native system file picker)
	fc.Btn = widget.NewButton("Upload Folder", func() {
		dir, err := zenity.SelectFile(zenity.Directory(), zenity.Title("Choose a folder"))
		if errors.Is(err, zenity.ErrCanceled) {
			return
		}
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		// Create URI for accessing through Fyne
		u := storage.NewFileURI(dir)
		listable, err := storage.ListerForURI(u)
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		children, err := listable.List()
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		// Loop through files in folder to store in FileCell
		images, warning, err := WalkChildren(children)
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		if warning {
			resize := WarningWindow(a, dir, func() {
				if len(images) == 0 {
					imgError := errors.New("No images found. Please check the file format of the uploaded images. Accepted file types are '.png,', '.jpg', '.jpeg'")
					dialog.ShowError(imgError, parent)
					return
				}

				// Sort files in slice of file paths
				sort.SliceStable(images, func(i, j int) bool {
					ni, okI := FrameIndex(images[i], fc.Regex)
					nj, okJ := FrameIndex(images[j], fc.Regex)

					if okI && okJ && ni != nj {
						return ni < nj
					}
					if okI != okJ {
						// Files with numeric suffix come before those without
						return okI
					}
					// Fallback by basename (case-insensitive)
					return strings.ToLower(images[i].Name()) < strings.ToLower(images[j].Name())
				})

				fc.FilesMu.Lock()
				fc.Files = images
				fc.FilesMu.Unlock()

				if fc.OnLoaded != nil {
					fc.OnLoaded(len(images))
				}

				_ = fc.Show(0)
				fc.Btn.Hide()
			})
			resize.CenterOnScreen()
			resize.Show()
			return
		}

		if len(images) == 0 {
			imgError := errors.New("No images found. Please check the file format of the uploaded images. Accepted file types are '.png,', '.jpg', '.jpeg'")
			dialog.ShowError(imgError, parent)
			return
		}

		// Sort files in slice of file paths
		sort.SliceStable(images, func(i, j int) bool {
			ni, okI := FrameIndex(images[i], fc.Regex)
			nj, okJ := FrameIndex(images[j], fc.Regex)

			if okI && okJ && ni != nj {
				return ni < nj
			}
			if okI != okJ {
				// Files with numeric suffix come before those without
				return okI
			}
			// Fallback by basename (case-insensitive)
			return strings.ToLower(images[i].Name()) < strings.ToLower(images[j].Name())
		})

		//DEBUG TO CHECK SORT ORDER
		// for i := 0; i < len(images); i++ {
		// 	fmt.Printf("[DEBUG] FILES: %s\n", images[i].Name())
		// }

		fc.FilesMu.Lock()
		fc.Files = images
		fc.FilesMu.Unlock()

		if fc.OnLoaded != nil {
			fc.OnLoaded(len(images))
		}

		_ = fc.Show(0)
		fc.Btn.Hide()
	})

	// Make sure button is centered in the rectangle
	upload := container.NewBorder(
		layout.NewSpacer(),
		layout.NewSpacer(),
		layout.NewSpacer(),
		layout.NewSpacer(),
		fc.Btn,
	)

	// Stack centered button ontop of rectangle
	fc.Container = container.NewStack(fc.Background, upload)

	return fc
}

// Helper function utilizing similar logic to NewFileCell to refresh images shown in a cell. To be used when frame regex is changed
func RefreshFileCell(cell *FileCell, idx int) {
	images := cell.Files
	// Sort files in slice of file paths
	sort.SliceStable(images, func(i, j int) bool {
		ni, okI := FrameIndex(images[i], cell.Regex)
		nj, okJ := FrameIndex(images[j], cell.Regex)

		if okI && okJ && ni != nj {
			return ni < nj
		}
		if okI != okJ {
			// Files with numeric suffix come before those without
			return okI
		}
		// Fallback by basename (case-insensitive)
		return strings.ToLower(images[i].Name()) < strings.ToLower(images[j].Name())
	})

	//DEBUG TO CHECK SORT ORDER
	for i := range len(images) {
		fmt.Printf("[DEBUG] Cell[%d]: %s\n", idx, images[i].Name())
	}

	cell.FilesMu.Lock()
	cell.Files = images
	cell.FilesMu.Unlock()

	if cell.OnLoaded != nil {
		cell.OnLoaded(len(images))
	}
	cell.Container.Refresh()
}

// Helper function to build filename label inside the info window (dynamically color segments based on regexp pattern)
func BuildFileLabel(cell *FileCell, pString string) (*fyne.Container, error) {
	var file *widget.TextSegment
	var fileLabel *fyne.Container

	style := widget.RichTextStyle{
		Inline:    true,
		SizeName:  theme.SizeNameCaptionText,
		TextStyle: fyne.TextStyle{Monospace: true},
	}

	pattern, err := regexp.Compile(pString)
	if err != nil {
		fmt.Println("[LOG] Not valid regex...")
		file = &widget.TextSegment{Style: style, Text: cell.Files[0].Name()}
		rt := widget.NewRichText(file)
		fileLabel = container.NewBorder(nil, nil, layout.NewSpacer(), layout.NewSpacer(), rt)
		return fileLabel, nil
	}

	frameNo, ok := FrameIndex(cell.Files[0], pattern)
	idx := strings.Index(cell.Files[0].Name(), strconv.Itoa(frameNo))
	if !ok || idx == -1 {
		if len(cell.Files[0].Name()) > 47 {
			file = &widget.TextSegment{Style: style, Text: cell.Files[0].Name()[:45]} // 45 since adding 3 chars with '...' = 48 although [:x] is non inclusive so its 47 char long which we want to be max
		} else {
			file = &widget.TextSegment{Style: style, Text: cell.Files[0].Name()}
		}
		rt := widget.NewRichText(file)
		fileLabel = container.NewBorder(nil, nil, layout.NewSpacer(), layout.NewSpacer(), rt)
	} else {
		fileText1 := &widget.TextSegment{Style: style, Text: cell.Files[0].Name()[:idx]}

		frame := &widget.TextSegment{Style: widget.RichTextStyle{Inline: true, SizeName: theme.SizeNameCaptionText, TextStyle: fyne.TextStyle{Monospace: true}, ColorName: theme.ColorNameError}, Text: strconv.Itoa(frameNo)} // Show the match from regex in red (should be frame number)

		fileText2 := &widget.TextSegment{Style: style, Text: cell.Files[0].Name()[idx+len(strconv.Itoa(frameNo)):]}

		rt := widget.NewRichText(fileText1, frame, fileText2)
		fileLabel = container.NewBorder(nil, nil, layout.NewSpacer(), layout.NewSpacer(), rt)

		//fmt.Printf("[DEBUG] fileText1: %s\nframe: %s\nfileText2: %s\n", fileText1.Text, frame.Text, fileText2.Text)
	}
	return fileLabel, nil
}

// UI function to build file size warning window
func WarningWindow(a fyne.App, inputDir string, onCancel func()) fyne.Window {
	w := a.NewWindow("File Size Warning")
	w.Resize(fyne.NewSize(500, 200))
	w.SetFixedSize(true)

	// Header
	msg := canvas.NewText("One or more of your images is larger than 1 MB", ui.Giffy)
	msg.TextSize = 16
	msg.Alignment = fyne.TextAlignCenter

	headerSpacer := NewSpacer(10, 15)

	q := canvas.NewText("Resize all images with Giffy?", ui.Giffy)
	q.TextSize = 16
	q.Alignment = fyne.TextAlignCenter

	headerV := container.NewVBox(headerSpacer, msg)
	header := container.NewHBox(layout.NewSpacer(), headerV, layout.NewSpacer())

	// Button area
	cancel := widget.NewButton("Cancel", func() {
		w.Close()
		if onCancel != nil {
			onCancel()
		}
	})

	ok := widget.NewButton("  OK  ", func() {
		ResizePNG(w, inputDir)
	})

	spacer := NewSpacer(20, 25)
	sp2 := NewSpacer(0, 5)

	btnAreaH := container.NewHBox(layout.NewSpacer(), cancel, spacer, ok, layout.NewSpacer())
	btnArea := container.NewVBox(layout.NewSpacer(), q, spacer, btnAreaH, sp2)

	footerSpacer := NewSpacer(10, 25)

	w.SetContent(container.NewBorder(
		header,
		footerSpacer,
		nil,
		nil,
		btnArea,
	))

	return w
}

// UI function to allow user to specify file resize folder
func ResizePNG(parent fyne.Window, inputDir string) {
	var content *fyne.Container
	var opts resize.ResizeOptions
	var uploaded bool = false
	var setContent func() fyne.CanvasObject
	var err error

	opts.InputDir = inputDir

	// Before Uploading
	title := canvas.NewText("Please specify path for resized images", ui.Giffy)
	title.TextSize = 16
	title.Alignment = fyne.TextAlignCenter

	spacer := NewSpacer(10, 20)

	btn := widget.NewButton("Select Folder", func() {
		opts.OutputDir, err = zenity.SelectFile(zenity.Directory(), zenity.Title("Choose a folder"))
		if errors.Is(err, zenity.ErrCanceled) {
			return
		}
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		uploaded = true
		parent.SetContent(setContent())

		fmt.Printf("Resize dir: %s\n", opts.OutputDir)
	})

	// After Uploading
	title2 := canvas.NewText("Please specify maximum width and height", ui.Giffy)
	title2.TextSize = 16
	title2.Alignment = fyne.TextAlignCenter

	t2 := canvas.NewText("(Recommended MAX is 900W & 900H)", ui.Giffy)
	t2.TextSize = 11
	t2.Alignment = fyne.TextAlignCenter

	sp2 := NewSpacer(10, 10)

	entryWidth := NewSpacer(150, 0)

	widthInput := widget.NewEntry()
	widthInput.SetPlaceHolder("Enter width...")
	heightInput := widget.NewEntry()
	heightInput.SetPlaceHolder("Enter height...")

	isNumeric := func(s string) bool {
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	}

	submitBtn := widget.NewButton("Resize", func() {
		if widthInput.Text != "" && heightInput.Text != "" {
			opts.MaxW, err = strconv.Atoi(widthInput.Text)
			if err != nil {
				dialog.ShowError(err, parent)
				return
			}

			opts.MaxH, err = strconv.Atoi(heightInput.Text)
			if err != nil {
				dialog.ShowError(err, parent)
				return
			}
		}

		if opts.MaxW < 0 || !isNumeric(widthInput.Text) || widthInput.Text == "" || opts.MaxH < 0 || !isNumeric(heightInput.Text) || heightInput.Text == "" {
			dialog.ShowInformation("Invalid Entry", "Please enter valid maximum width and height", parent)
		} else {
			fmt.Println("[DEBUG] Starting resizing...")
			progress := dialog.NewCustomWithoutButtons("Resizing...", &widget.ProgressBarInfinite{}, parent)
			progress.Show()
			go func() {
				processed, resized, errors := resize.ProcessDir(opts)
				fyne.Do(func() {
					progress.Hide()
					info := dialog.NewInformation("Resize Complete", fmt.Sprintf("Resize Completed\nProcessed: %d, Resized: %d, Errors: %d", processed, resized, errors), parent)
					info.SetOnClosed(func() { parent.Close() })
					info.Show()
				})
			}()
		}
	})

	setContent = func() fyne.CanvasObject {
		if !uploaded {
			contentV := container.NewVBox(layout.NewSpacer(), title, spacer, container.NewHBox(layout.NewSpacer(), btn, layout.NewSpacer()), layout.NewSpacer())
			content = container.NewHBox(layout.NewSpacer(), contentV, layout.NewSpacer())
			fmt.Printf("[DEBUG] Content Set, Uploaded = %t\n", uploaded)
		} else {
			// Fyne craziness
			widthBox := container.NewVBox(entryWidth, widthInput)
			heightBox := container.NewVBox(entryWidth, heightInput)
			contentV := container.NewVBox(layout.NewSpacer(), title2, sp2, t2, sp2, container.NewHBox(layout.NewSpacer(), widthBox, spacer, heightBox, layout.NewSpacer()), sp2, container.NewHBox(layout.NewSpacer(), submitBtn, layout.NewSpacer()), layout.NewSpacer())
			content = container.NewHBox(layout.NewSpacer(), contentV, layout.NewSpacer())
		}
		return content
	}

	parent.SetContent(setContent())

}

// Helper function to make specifically sized spacer objects
func NewSpacer(width, height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(width, height))

	return spacer
}

// Helper function to walk the folder provided by user and 1) Only track images and 2) Raise a warning if the image is > 1 MB
func WalkChildren(children []fyne.URI) ([]fyne.URI, bool, error) {
	fileSizeWarning := false
	var images []fyne.URI

	for _, c := range children {
		// Only add image files (avoid .DS_Store like the plague)
		e := strings.ToLower(filepath.Ext(c.Name()))
		switch e {
		case ".png", ".jpg", ".jpeg":
			//fmt.Printf("[DEBUG] Added File: %s\n", c)

			// Load image path with os.Stat to retrieve file size in bytes
			if e == ".png" {
				imageInfo, err := os.Stat(c.Path())
				if err != nil {
					return nil, false, fmt.Errorf("imageInfo failed: %v", err)
				}

				// Convert bytes to megabytes
				imageSize := float64(float64(imageInfo.Size()) / (1024.00 * 1024.00))

				// Trigger warning if file size is greater than 2 MB
				if imageSize > 1.0 {
					fileSizeWarning = true
				} else {
					//	fmt.Printf("[DEBUG] Image Size OK: %v\n", imageSize)
				}
			}

			images = append(images, c)
		}
	}
	return images, fileSizeWarning, nil
}

// Helper function to return trailing numeric values in filename
func FrameIndex(u fyne.URI, pattern *regexp.Regexp) (int, bool) {
	name := u.Name()
	base := strings.TrimSuffix(name, filepath.Ext(name))
	m := pattern.FindStringSubmatch(base)
	if m == nil || len(m) == 1 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Function to build screen shots of each frame on the grid and develop a .gif
func BuildExport(dest fyne.URI, rows, cols int, cells []*FileCell, fps int, _ *widget.Slider) error {
	if len(cells) == 0 {
		return errors.New("no cells to export")
	}
	if rows*cols != len(cells) {
		return fmt.Errorf("rows*cols (%d) != cells (%d)", rows*cols, len(cells))
	}
	if fps <= 0 {
		fps = 15
	}

	ffmpegPath, err := EnsureFFmpeg()
	if err != nil {
		return fmt.Errorf("ffmpeg unavailable: %w", err)
	}
	ffmpegDir := filepath.Dir(ffmpegPath)
	pathEnv := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	if !strings.Contains(sep+pathEnv+sep, sep+ffmpegDir+sep) {
		_ = os.Setenv("PATH", ffmpegDir+sep+pathEnv)
	}

	// Validating output folder and saving base for file names
	outDir := dest.Path()
	if outDir == "" {
		return errors.New("invalid destination")
	}
	outBase := filepath.Base(outDir)

	// Determine total frames (cells should all have same count, guard against 0)
	maxFrames := cells[0].Len()
	if maxFrames <= 0 {
		return errors.New("no frames loaded")
	}

	// Decide target cell size from first frame dimensions across cells
	cellW, cellH := 0, 0
	for _, c := range cells {
		if c.Len() == 0 {
			continue
		}
		img, err := DecodeURI(c.Files[0])
		if err != nil || img == nil {
			continue
		}
		b := img.Bounds()
		if b.Dx() > cellW {
			cellW = b.Dx()
		}
		if b.Dy() > cellH {
			cellH = b.Dy()
		}
	}
	if cellW == 0 || cellH == 0 {
		return errors.New("could not determine cell size")
	}

	// Make canvas size
	outW := cols * cellW
	outH := rows * cellH

	// Ensure filenames look nice
	pad := len(fmt.Sprintf("%d", maxFrames-1))

	// Render & save PNG frames
	for f := 0; f < maxFrames; f++ {
		// RGBA canvas for PNG export
		rgba := image.NewRGBA(image.Rect(0, 0, outW, outH))

		// Draw white background
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{C: color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

		// Draw each cell’s frame at [r, c]
		for r := 0; r < rows; r++ {
			for cIdx := 0; cIdx < cols; cIdx++ {
				cell := cells[r*cols+cIdx]
				if cell.Len() == 0 {
					continue
				}
				frameIdx := f % cell.Len()
				srcImg, err := DecodeURI(cell.Files[frameIdx])
				if err != nil || srcImg == nil {
					continue
				}
				dstRect := image.Rect(cIdx*cellW, r*cellH, (cIdx+1)*cellW, (r+1)*cellH)
				xdraw.CatmullRom.Scale(rgba, dstRect, srcImg, srcImg.Bounds(), xdraw.Over, nil)
			}
		}

		// Write PNG frame
		pngName := filepath.Join(outDir, fmt.Sprintf("%s_%0*d.png", outBase, pad, f))
		if err := WritePNG(pngName, rgba); err != nil {
			return fmt.Errorf("png frame %d: %w", f, err)
		}
	}

	// Build encoder args
	encArgs := []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "14", "-preset", "slow", "-tune", "animation"}

	// Encode MP4 with ffmpeg using the numbered PNGs
	pattern := fmt.Sprintf("%s_%%0%dd.png", outBase, pad)
	mp4Name := fmt.Sprintf("%s.mp4", outBase)

	args := []string{
		"-y",
		"-framerate", strconv.Itoa(fps),
		"-i", pattern,
	}
	args = append(args, encArgs...)
	args = append(args,
		"-movflags", "+faststart",
		mp4Name,
	)

	var out bytes.Buffer
	cmd := exec.Command(ffmpegPath, args...)
	cmd.Dir = outDir
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		// If hardware encode failed, fall back to libx264 automatically once
		if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
			fallback := []string{
				"-y", "-framerate", strconv.Itoa(fps),
				"-i", pattern,
				"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "14", "-preset", "slow", "-tune", "animation",
				"-movflags", "+faststart",
				mp4Name,
			}
			out.Reset()
			cmd = exec.Command(ffmpegPath, fallback...)
			cmd.Dir = outDir
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err2 := cmd.Run(); err2 != nil {
				return fmt.Errorf("ffmpeg encode failed: %v\n%s", err2, out.String())
			}
		} else {
			return fmt.Errorf("ffmpeg encode failed: %v\n%s", err, out.String())
		}
	}
	return nil
}

// Helper function to read image files in the FileCell slice of files
func DecodeURI(u fyne.URI) (image.Image, error) {
	key := u.String()

	// Return from cache if available
	ImgCache.RLock()
	if img, ok := ImgCache.m[key]; ok && img != nil {
		ImgCache.RUnlock()
		return img, nil
	}
	ImgCache.RUnlock()

	// Decode and cache
	rc, err := storage.Reader(u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, err
	}

	ImgCache.Lock()
	ImgCache.m[key] = img
	ImgCache.Unlock()

	return img, nil
}

func WritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func EnsureFFmpeg() (string, error) {
	var bin []byte
	var name string

	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			bin = ffmpegDarwinArm64
		} else {
			bin = ffmpegDarwinAmd64
		}
		name = "ffmpeg"
	case "windows":
		bin = ffmpegWindowsAmd64
		name = "ffmpeg.exe"
	case "linux":
		bin = ffmpegLinuxAmd64
		name = "ffmpeg"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if len(bin) == 0 {
		return "", errors.New("ffmpeg binary not embedded for this platform")
	}

	// Write to a cache dir with a content hash in the filename to avoid stale copies.
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(cacheRoot, "giffy", "bin")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}

	sum := fmt.Sprintf("%x", sha256.Sum256(bin))[:16]
	outPath := filepath.Join(appDir, fmt.Sprintf("%s-%s", sum, name))

	// If already exists with same hash, reuse it.
	if st, err := os.Stat(outPath); err == nil && st.Size() == int64(len(bin)) {
		return outPath, nil
	}

	// Write (atomic-ish)
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil { // 755 ensures exec bit on unix
		return "", err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return "", err
	}

	// On Windows, perms are handled by ACLs; on Unix we ensure executable bit:
	if runtime.GOOS != "windows" {
		_ = os.Chmod(outPath, 0o755)
	}
	return outPath, nil
}
