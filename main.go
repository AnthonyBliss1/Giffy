package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"
	xdraw "golang.org/x/image/draw"

	"github.com/anthonybliss1/giffy/gui/utils"
	ui "github.com/anthonybliss1/giffy/theme"
)

//go:embed assets
var embeddedAssets embed.FS

//go:embed assets/ffmpeg/darwin-arm64/ffmpeg
var ffmpegDarwinArm64 []byte

//go:embed assets/ffmpeg/darwin-amd64/ffmpeg
var ffmpegDarwinAmd64 []byte

//go:embed assets/ffmpeg/windows-amd64/ffmpeg.exe
var ffmpegWindowsAmd64 []byte

//go:embed assets/ffmpeg/linux-amd64/ffmpeg
var ffmpegLinuxAmd64 []byte

var playIcon, pauseIcon, downloadIcon, iconLogo fyne.Resource

var reTrailingDigits = regexp.MustCompile(`(\d+)$`)

// Wrapping cell components in a struct for easy access
type FileCell struct {
	Container  *fyne.Container
	Background *canvas.Rectangle
	Btn        *widget.Button
	Img        *canvas.Image
	Files      []fyne.URI
	filesMu    sync.RWMutex

	OnLoaded func(count int) // Will be the max frames for the slider
}

// Struct helper function to return the length of the slice of file paths
func (fc *FileCell) Len() int {
	fc.filesMu.RLock()
	defer fc.filesMu.RUnlock()

	return len(fc.Files)
}

// Struct helper function to create a canvas image object from a specific index in the slice of file paths
func (fc *FileCell) Show(i int) error {
	fc.filesMu.RLock()
	defer fc.filesMu.RUnlock()

	if i < 0 || i >= len(fc.Files) {
		return fmt.Errorf("index for image file out of range: %d (LENGTH: %d)", i, len(fc.Files))
	}

	// Load URI Path
	u := fc.Files[i]

	// Create image canvas object, add it to FileCell struct, then add to File Cell Container
	if fc.Img == nil {
		fc.Img = canvas.NewImageFromURI(u)
		fc.Img.FillMode = canvas.ImageFillContain
		fc.Background.FillColor = color.Black
		fc.Container.Objects = []fyne.CanvasObject{fc.Background, fc.Img}
	} else {
		newImg := canvas.NewImageFromURI(u)
		newImg.FillMode = canvas.ImageFillContain
		fc.Img = newImg
		fc.Background.FillColor = color.Black
		fc.Container.Objects = []fyne.CanvasObject{fc.Background, fc.Img}
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

// Need to load the svg files for Play & Pause buttons
func init() {
	play, err := embeddedAssets.ReadFile("assets/play.svg")
	if err != nil {
		log.Panic("failed to load play.svg from assets file")
	}
	playIcon = fyne.NewStaticResource("assets/play.svg", play)

	pause, err := embeddedAssets.ReadFile("assets/pause.svg")
	if err != nil {
		log.Panic("failed to load pause.svg from assets file")
	}
	pauseIcon = fyne.NewStaticResource("assets/pause.svg", pause)

	download, err := embeddedAssets.ReadFile("assets/download.svg")
	if err != nil {
		log.Panic("failed to load download.svg from assets file")
	}
	downloadIcon = fyne.NewStaticResource("assets/download.svg", download)

	icon, err := embeddedAssets.ReadFile("assets/icon.png")
	if err != nil {
		log.Panic("failed to load icon.png from assets file")
	}
	iconLogo = fyne.NewStaticResource("assets/icon.png", icon)
}

func main() {
	a := app.New()

	base := theme.DefaultTheme()
	a.Settings().SetTheme(&ui.ForcedVariant{
		Theme:   base,
		Variant: theme.VariantDark,
	})

	a.SetIcon(iconLogo)

	config := configWindow(a)
	config.CenterOnScreen()
	config.ShowAndRun()
}

// Config screen to set the desired size of the grid of images
func configWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("Giffy")
	w.Resize(fyne.NewSize(600, 300))
	w.SetFixedSize(true)

	// Header (Title & Version Number)
	topSpacer := newSpacer(1, 5)
	title := canvas.NewText("Giffy", ui.Giffy)
	title.TextSize = 30
	title.Alignment = fyne.TextAlignCenter

	versionNumber := canvas.NewText("v1.0", ui.Giffy)
	versionNumber.TextSize = 14
	versionNumber.Alignment = fyne.TextAlignCenter

	header := container.NewVBox(topSpacer, title, versionNumber)

	// Config Inputs for Grid Layout
	layoutSpacer := newSpacer(75, 52) // 52 height to put in line with 'X'
	labelSpacer := newSpacer(5, 5)

	// Row Entry
	gridRows := widget.NewEntry()
	gridRowsLabel := canvas.NewText("Rows", ui.Giffy)
	gridRowsLabel.TextSize = 14
	gridRowsLabel.Alignment = fyne.TextAlignCenter
	rowConfig := container.NewVBox(layoutSpacer, gridRows, labelSpacer, gridRowsLabel)

	// Column Entry
	gridColumns := widget.NewEntry()
	gridColumnsLabel := canvas.NewText("Columns", ui.Giffy)
	gridColumnsLabel.TextSize = 14
	gridColumnsLabel.Alignment = fyne.TextAlignCenter
	columnsConfig := container.NewVBox(layoutSpacer, gridColumns, labelSpacer, gridColumnsLabel)

	// Middle 'X' column between the two inputs
	xlabel := canvas.NewText("X", ui.Giffy)
	xlabel.TextSize = 30
	xSpacer := newSpacer(1, 50)
	xColumn := container.NewVBox(xSpacer, xlabel, xSpacer)

	// Space to separate Row Entry, 'X', and Column Entry
	configSpacer := newSpacer(60, 5)
	gridConfig := container.NewHBox(layout.NewSpacer(), rowConfig, configSpacer, xColumn, configSpacer, columnsConfig, layout.NewSpacer())

	// helper function to check user input
	isNumeric := func(s string) bool {
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	}

	// Button
	buildGrid := widget.NewButton("  Build  ", func() {
		r, _ := strconv.Atoi(gridRows.Text)
		c, _ := strconv.Atoi(gridColumns.Text)
		if gridRows.Text == "" || r <= 0 || !isNumeric(gridRows.Text) || gridColumns.Text == "" || c <= 0 || !isNumeric(gridColumns.Text) {
			dialog.ShowInformation("Incomplete Entry", "Please enter valid numbers for the rows and columns", w)
		} else if r > 5 || c > 5 {
			dialog.ShowInformation("Invalid Entry", "Maximum of 5 rows and 5 columns is allowed", w)
		} else {
			gridRowsInt, _ := strconv.Atoi(gridRows.Text)
			gridColumnsInt, _ := strconv.Atoi(gridColumns.Text)
			grid := gridWindow(a, gridRowsInt, gridColumnsInt)
			w.Hide()
			grid.CenterOnScreen()
			grid.Show()
			w.Close()
		}
	})

	// Space at bottom of window
	footerSpacer := newSpacer(1, 20)

	// Need to wrap a vertical box in a horz box because this framework gives me no other option to center :|
	buttonArea := container.NewHBox(layout.NewSpacer(), container.NewVBox(layout.NewSpacer(), buildGrid, footerSpacer), layout.NewSpacer())

	w.SetContent(container.NewBorder(
		header,
		buttonArea,
		nil,
		nil,
		gridConfig))

	return w
}

// The main work area
func gridWindow(a fyne.App, gridRows, gridColumns int) fyne.Window {
	// Create new gridWindow
	w := a.NewWindow("Giffy")
	w.Resize(fyne.NewSize(800, 500))
	w.SetFixedSize(false)

	// Create frame variable and binding to attach to slider
	var frameFloat float64 = 0
	var slider *widget.Slider
	var fpsInput *widget.Entry
	cells := make([]*FileCell, 0, gridRows*gridColumns)

	currentFrame := binding.BindFloat(&frameFloat)
	currentFrame.Set(frameFloat)

	// Helper function to drag and drop parsing FPS input
	parseFPS := func(_ string) int {
		v, err := strconv.Atoi(fpsInput.Text)
		if err != nil || v <= 0 {
			return 5
		}
		if v > 15 {
			return 15
		}
		return v
	}

	// Header Area (Title)
	title := canvas.NewText("Giffy", ui.Giffy)
	title.TextSize = 18
	title.Alignment = fyne.TextAlignCenter

	// Show current frame number
	frames := canvas.NewText(fmt.Sprintf("Frame: %.0f", frameFloat), ui.Giffy)
	frames.TextSize = 14
	frames.Alignment = fyne.TextAlignCenter

	// Put title and frames in a vertical box to manage spacing
	comboSpacer := newSpacer(1, 10)
	combo := container.NewVBox(newSpacer(1, 2), title, frames, comboSpacer)

	// Put title and frame together for the header
	titleHeader := container.NewHBox(layout.NewSpacer(), combo, layout.NewSpacer())

	// Add Build button area
	downloadBtn := widget.NewButtonWithIcon("", downloadIcon, func() {
		// Chose export folder from zenity file picker (only can chose a directory)
		if slider.Max != 0 {
			dir, err := zenity.SelectFile(zenity.Directory(), zenity.Title("Choose export folder"))
			if errors.Is(err, zenity.ErrCanceled) {
				return
			}
			if err != nil {
				dialog.ShowError(err, w)
				return
			}

			// Create URI for accessing through Fyne
			u := storage.NewFileURI(dir)

			fps := parseFPS(fpsInput.Text)

			// run export off the UI thread with a simple progress dialog
			progress := dialog.NewCustomWithoutButtons("Exporting...", &widget.ProgressBarInfinite{}, w)
			progress.Show()
			go func() {
				err := buildExport(u, gridRows, gridColumns, cells, fps, slider)
				fyne.Do(func() {
					progress.Hide()
					if err != nil {
						dialog.ShowError(err, w)
					} else {
						dialog.ShowInformation("Done", "Export complete!", w)
					}
				})
			}()
		} else {
			dialog.ShowInformation("Add Files", "Please add files to the grid before exporting", w)
		}
	})
	buildBtnArea := container.NewHBox(layout.NewSpacer(), container.NewVBox(newSpacer(10, 9), downloadBtn), newSpacer(110, 10))

	// FPS input for header hbox
	fpsInput = widget.NewEntry()
	fpsInput.SetText("5") // Default 5 FPS
	fpsLabel := canvas.NewText("FPS", ui.Giffy)
	fpsLabel.TextSize = 16
	fpsSpacer := newSpacer(40, 10)
	margin := newSpacer(10, 17)

	fpsBox := container.NewHBox(layout.NewSpacer(), container.NewVBox(fpsSpacer, fpsInput), container.NewVBox(margin, fpsLabel), margin)

	// Add everything to header (two hboxes stacked so title and frames stay centered)
	header := container.NewStack(titleHeader, buildBtnArea, fpsBox)

	// Initialize grid container
	gridArea := container.New(layout.NewGridLayout(gridColumns))

	// Add the slider at the bottom (max value will need to be the number of files in the folders ie. total frames)
	slider = widget.NewSliderWithData(0, 0, currentFrame)
	slider.Step = 1
	sliderSpacer := newSpacer(1, 15)

	// Add proper amount of cells to gridArea
	for i := 0; i < (gridColumns * gridRows); i++ {
		cell := newFileCell(a, w)
		cell.OnLoaded = func(count int) {
			slider.Max = float64(count)
		}
		cell.BindToIndex(currentFrame)
		gridArea.Add(cell.Container)
		cells = append(cells, cell)
	}

	// Manage state when slider is changing
	slider.OnChanged = func(v float64) {
		frames.Text = fmt.Sprintf("Frame: %.0f", v)
		currentFrame.Set(v)
		frames.Refresh()
	}

	// Add some margin to the content on the left and right side
	spacer := newSpacer(10, 1)

	// Add a play/pause button to play all frame instead of sliding through them
	isPlaying := false
	var controlBtn *widget.Button
	var cancel context.CancelFunc
	fpsChan := make(chan time.Duration, 1)

	// FPS input on change function to recalc the FPS and apply it
	fpsInput.OnChanged = func(_ string) {
		if !isPlaying {
			return
		}
		fps := parseFPS(fpsInput.Text)
		ms := time.Second / time.Duration(fps)
		fmt.Printf("[DEBUG] MS = %v\n", ms)

		select {
		case fpsChan <- ms:
		default:
			<-fpsChan
			fpsChan <- ms
		}
	}

	controlBtn = widget.NewButtonWithIcon("", playIcon, func() {
		if !isPlaying && slider.Max != 0 {
			fmt.Println("[DEBUG] Playing gifs")
			isPlaying = true
			fyne.Do(func() { controlBtn.SetIcon(pauseIcon) })

			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())

			// Play the gif by looping through the frames
			go func() {
				// Calc milliseconds from FPS input
				fps := parseFPS(fpsInput.Text)
				ms := time.Second / time.Duration(fps)
				fmt.Printf("[DEBUG] MS = %v\n", ms)
				t := time.NewTicker(ms)
				defer t.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case newMS := <-fpsChan:
						if newMS <= 0 {
							continue
						}
						t.Stop()
						t = time.NewTicker(newMS)

					case <-t.C:
						v, _ := currentFrame.Get()
						v++

						// reset counter to loop until canceled
						if v > slider.Max {
							v = 0
						}
						_ = currentFrame.Set(v)
					}
				}
			}()
		} else if isPlaying {
			fmt.Println("[DEBUG] Pausing gifs")
			isPlaying = false
			if cancel != nil {
				cancel()
				cancel = nil
			}
			fyne.Do(func() { controlBtn.SetIcon(playIcon) })
		}
	})

	// Create button area to add spacer to the right
	controlBtnArea := container.NewHBox(controlBtn, spacer)

	// Combine slider and play/pause
	bottomControls := container.NewBorder(sliderSpacer, sliderSpacer, nil, controlBtnArea, slider)

	w.SetContent(
		container.NewBorder(
			header,
			bottomControls,
			spacer,
			spacer,
			gridArea,
		))

	return w
}

// Helper function to create each cell in the grid layout
func newFileCell(a fyne.App, parent fyne.Window) *FileCell {
	// Initialize collection struct
	fc := &FileCell{}

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
		images, warning, err := walkChildren(children)
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		if warning {
			resize := warningWindow(a, dir)
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
			ni, okI := frameIndex(images[i])
			nj, okJ := frameIndex(images[j])

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

		fc.filesMu.Lock()
		fc.Files = images
		fc.filesMu.Unlock()

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

// UI function to build file size warning window
func warningWindow(a fyne.App, inputDir string) fyne.Window {
	w := a.NewWindow("File Size Warning")
	w.Resize(fyne.NewSize(500, 200))
	w.SetFixedSize(true)

	// Header
	msg := canvas.NewText("One or more of your images is larger than 2 MB", ui.Giffy)
	msg.TextSize = 16
	msg.Alignment = fyne.TextAlignCenter

	headerSpacer := newSpacer(10, 15)

	q := canvas.NewText("Resize all images with Giffy?", ui.Giffy)
	q.TextSize = 16
	q.TextStyle.Bold = true
	q.Alignment = fyne.TextAlignCenter

	headerV := container.NewVBox(headerSpacer, msg)
	header := container.NewHBox(layout.NewSpacer(), headerV, layout.NewSpacer())

	// Button area
	cancel := widget.NewButton("Cancel", func() {
		w.Close()
	})

	ok := widget.NewButton("  OK  ", func() {
		resizePNG(w, inputDir)
	})

	spacer := newSpacer(20, 20)

	btnAreaH := container.NewHBox(layout.NewSpacer(), cancel, spacer, ok, layout.NewSpacer())
	btnArea := container.NewVBox(layout.NewSpacer(), q, spacer, btnAreaH)

	footerSpacer := newSpacer(10, 25)

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
func resizePNG(parent fyne.Window, inputDir string) {
	var content *fyne.Container
	var opts utils.ResizeOptions
	var uploaded bool = false
	var setContent func() fyne.CanvasObject
	var outputDir string
	var err error

	opts.InputDir = inputDir

	// Before Uploading
	title := canvas.NewText("Please specify path for resized images", ui.Giffy)
	title.TextSize = 14
	title.Alignment = fyne.TextAlignCenter

	spacer := newSpacer(10, 20)

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

		fmt.Printf("Resize dir: %s\n", outputDir)
	})

	// After Uploading
	title2 := canvas.NewText("Please specify maximum width and height", ui.Giffy)
	title2.TextSize = 14
	title2.Alignment = fyne.TextAlignCenter

	entryWidth := newSpacer(150, 0)

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
				processed, resized, errors := utils.ProcessDir(opts)
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
			contentV := container.NewVBox(layout.NewSpacer(), title2, spacer, container.NewHBox(layout.NewSpacer(), widthBox, spacer, heightBox, layout.NewSpacer()), spacer, container.NewHBox(layout.NewSpacer(), submitBtn, layout.NewSpacer()), layout.NewSpacer())
			content = container.NewHBox(layout.NewSpacer(), contentV, layout.NewSpacer())
		}
		return content
	}

	parent.SetContent(setContent())

}

// Helper function to make specifically sized spacer objects
func newSpacer(width, height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(width, height))

	return spacer
}

// Helper function to walk the folder provided by user and 1) Only track images and 2) Raise a warning if the image is > 2 MB
func walkChildren(children []fyne.URI) ([]fyne.URI, bool, error) {
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
				if imageSize > 2.0 {
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
func frameIndex(u fyne.URI) (int, bool) {
	name := u.Name()
	base := strings.TrimSuffix(name, filepath.Ext(name))
	m := reTrailingDigits.FindStringSubmatch(base)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Function to build screen shots of each frame on the grid and develop a .gif
func buildExport(dest fyne.URI, rows, cols int, cells []*FileCell, fps int, _ *widget.Slider) error {
	if len(cells) == 0 {
		return errors.New("no cells to export")
	}
	if rows*cols != len(cells) {
		return fmt.Errorf("rows*cols (%d) != cells (%d)", rows*cols, len(cells))
	}
	if fps <= 0 {
		fps = 15
	}

	ffmpegPath, err := ensureFFmpeg()
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
		img, err := decodeURI(c.Files[0])
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
				srcImg, err := decodeURI(cell.Files[frameIdx])
				if err != nil || srcImg == nil {
					continue
				}
				dstRect := image.Rect(cIdx*cellW, r*cellH, (cIdx+1)*cellW, (r+1)*cellH)
				xdraw.CatmullRom.Scale(rgba, dstRect, srcImg, srcImg.Bounds(), xdraw.Over, nil)
			}
		}

		// Write PNG frame
		pngName := filepath.Join(outDir, fmt.Sprintf("%s_%0*d.png", outBase, pad, f))
		if err := writePNG(pngName, rgba); err != nil {
			return fmt.Errorf("png frame %d: %w", f, err)
		}
	}

	// Build encoder args per OS
	encArgs := []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "18", "-preset", "medium"}

	switch runtime.GOOS {
	case "darwin":
		// macOS hardware
		encArgs = []string{"-c:v", "h264_videotoolbox", "-b:v", "6M", "-pix_fmt", "yuv420p"}
	case "windows":
		// Windows hardware
		encArgs = []string{"-c:v", "h264_mf", "-b:v", "6M", "-pix_fmt", "yuv420p"}
	}

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
				"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "18", "-preset", "medium",
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
func decodeURI(u fyne.URI) (image.Image, error) {
	rc, err := storage.Reader(u)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	return img, err
}

func writePNG(path string, img image.Image) error {
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

func ensureFFmpeg() (string, error) {
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
