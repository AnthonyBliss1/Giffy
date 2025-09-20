package windows

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	ui "github.com/anthonybliss1/giffy/theme"
	"github.com/ncruces/zenity"
)

//go:embed assets/icons
var embeddedAssets embed.FS

//go:embed assets/ffmpeg/darwin-arm64/ffmpeg
var ffmpegDarwinArm64 []byte

//go:embed assets/ffmpeg/darwin-amd64/ffmpeg
var ffmpegDarwinAmd64 []byte

//go:embed assets/ffmpeg/windows-amd64/ffmpeg.exe
var ffmpegWindowsAmd64 []byte

//go:embed assets/ffmpeg/linux-amd64/ffmpeg
var ffmpegLinuxAmd64 []byte

var reTrailingDigits = regexp.MustCompile(`(\d+)$`)

var newPattern *regexp.Regexp

var PlayIcon, PauseIcon, DownloadIcon, InfoIcon, IconLogo fyne.Resource

// Decode cache to avoid re-decoding frames repeatedly
var ImgCache = struct {
	sync.RWMutex
	m map[string]image.Image
}{
	m: make(map[string]image.Image),
}

// Need to load the svg files for Play & Pause buttons
func init() {
	play, err := embeddedAssets.ReadFile("assets/icons/play.svg")
	if err != nil {
		log.Panic("failed to load play.svg from assets file")
	}
	PlayIcon = fyne.NewStaticResource("assets/icons/play.svg", play)

	pause, err := embeddedAssets.ReadFile("assets/icons/pause.svg")
	if err != nil {
		log.Panic("failed to load pause.svg from assets file")
	}
	PauseIcon = fyne.NewStaticResource("assets/icons/pause.svg", pause)

	download, err := embeddedAssets.ReadFile("assets/icons/download.svg")
	if err != nil {
		log.Panic("failed to load download.svg from assets file")
	}
	DownloadIcon = fyne.NewStaticResource("assets/icons/download.svg", download)

	info, err := embeddedAssets.ReadFile("assets/icons/info.svg")
	if err != nil {
		log.Panic("failed to load info.svg from assets file")
	}
	InfoIcon = fyne.NewStaticResource("assets/icons/info.svg", info)

	icon, err := embeddedAssets.ReadFile("assets/icons/icon.png")
	if err != nil {
		log.Panic("failed to load icon.png from assets file")
	}
	IconLogo = fyne.NewStaticResource("assets/icons/icon.png", icon)
}

// Config screen to set the desired size of the grid of images
func ConfigWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("Giffy")
	w.Resize(fyne.NewSize(600, 300))
	w.SetFixedSize(true)

	// Header (Title & Version Number)
	topSpacer := NewSpacer(1, 5)
	title := canvas.NewText("Giffy", ui.Giffy)
	title.TextSize = 30
	title.Alignment = fyne.TextAlignCenter

	versionNumber := canvas.NewText("v1.0", ui.Giffy)
	versionNumber.TextSize = 14
	versionNumber.Alignment = fyne.TextAlignCenter

	header := container.NewVBox(topSpacer, title, versionNumber)

	// Config Inputs for Grid Layout
	layoutSpacer := NewSpacer(75, 52) // 52 height to put in line with 'X'
	labelSpacer := NewSpacer(5, 5)

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
	xSpacer := NewSpacer(1, 50)
	xColumn := container.NewVBox(xSpacer, xlabel, xSpacer)

	// Space to separate Row Entry, 'X', and Column Entry
	configSpacer := NewSpacer(60, 5)
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
			grid := GridWindow(a, gridRowsInt, gridColumnsInt)
			w.Hide()
			grid.CenterOnScreen()
			grid.Show()
			w.Close()
		}
	})

	// Space at bottom of window
	footerSpacer := NewSpacer(1, 20)

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
func GridWindow(a fyne.App, gridRows, gridColumns int) fyne.Window {
	// Create new gridWindow
	w := a.NewWindow("Giffy")
	w.Resize(fyne.NewSize(800, 500))
	w.SetFixedSize(false)
	w.SetOnClosed(func() { a.Quit() })

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
	comboSpacer := NewSpacer(1, 10)
	combo := container.NewVBox(NewSpacer(1, 2), title, frames, comboSpacer)

	// Put title and frame together for the header
	titleHeader := container.NewHBox(layout.NewSpacer(), combo, layout.NewSpacer())

	// Add Build button area
	downloadBtn := widget.NewButtonWithIcon("", DownloadIcon, func() {
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
				err := BuildExport(u, gridRows, gridColumns, cells, fps, slider)
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
	buildBtnArea := container.NewHBox(layout.NewSpacer(), container.NewVBox(NewSpacer(10, 9), downloadBtn), NewSpacer(110, 10))

	// FPS input for header hbox
	fpsInput = widget.NewEntry()
	fpsInput.SetText("5") // Default 5 FPS
	fpsLabel := canvas.NewText("FPS", ui.Giffy)
	fpsLabel.TextSize = 16
	fpsSpacer := NewSpacer(40, 10)
	margin := NewSpacer(10, 17)

	fpsBox := container.NewHBox(layout.NewSpacer(), container.NewVBox(fpsSpacer, fpsInput), container.NewVBox(margin, fpsLabel), margin)

	//Info button to see how th regex captured the frame numbers
	infoBtn := widget.NewButtonWithIcon("", InfoIcon, func() {
		if slider.Max != 0 {
			info := InfoWindow(a, cells)
			info.CenterOnScreen()
			info.Show()
		}
	})
	info := container.NewHBox(margin, container.NewVBox(NewSpacer(10, 9), infoBtn, NewSpacer(10, 10)), layout.NewSpacer())

	// Add everything to header (two hboxes stacked so title and frames stay centered)
	header := container.NewStack(titleHeader, buildBtnArea, fpsBox, info)

	// Initialize grid container
	gridArea := container.New(layout.NewGridLayout(gridColumns))

	// Add the slider at the bottom (max value will need to be the number of files in the folders ie. total frames)
	slider = widget.NewSliderWithData(0, 0, currentFrame)
	slider.Step = 1
	sliderSpacer := NewSpacer(1, 15)

	// Add proper amount of cells to gridArea
	for i := 0; i < (gridColumns * gridRows); i++ {
		cell := NewFileCell(a, w)
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
	spacer := NewSpacer(10, 1)

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

	controlBtn = widget.NewButtonWithIcon("", PlayIcon, func() {
		if !isPlaying && slider.Max != 0 {
			fmt.Println("[DEBUG] Playing gifs")
			isPlaying = true
			fyne.Do(func() { controlBtn.SetIcon(PauseIcon) })

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
			fyne.Do(func() { controlBtn.SetIcon(PlayIcon) })
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

// Function for the info button to show regex patterns
func InfoWindow(a fyne.App, cells []*FileCell) fyne.Window {
	w := a.NewWindow("Frame Patterns")
	w.Resize(fyne.NewSize(520, 320))
	w.SetFixedSize(true)

	// Header
	title := canvas.NewText("Edit Frame Identification", ui.Giffy)
	title.TextSize = 16
	title.Alignment = fyne.TextAlignCenter

	headerSpacer := NewSpacer(0, 10)

	header := container.NewVBox(headerSpacer, title, headerSpacer)

	// Grid Layout
	content := container.New(layout.NewVBoxLayout())
	gridArea := container.NewVScroll(content)
	spacerWidth1 := NewSpacer(410, 0)
	spacerWidth2 := NewSpacer(80, 0)
	spacer := NewSpacer(10, 5)
	margin := NewSpacer(15, 0)
	rowSpacer := NewSpacer(0, 10)

	// Creating info rows for each cell in the grid
	var entries = []*widget.Entry{}
	for i, cell := range cells {
		if len(cell.Files) == 0 {
			continue
		}

		label := canvas.NewText(fmt.Sprintf("Cell #%d", i+1), ui.Giffy)
		label.TextSize = 11

		fileCanvas := canvas.NewRectangle(color.Transparent)
		fileCanvas.StrokeColor = ui.Giffy
		fileCanvas.StrokeWidth = 1
		fileCanvas.CornerRadius = 4

		regPattern := widget.NewEntry()
		entries = append(entries, regPattern)
		regPattern.SetText(fmt.Sprintf("%v", cell.Regex))
		regLabel := canvas.NewText("RegEx", ui.Giffy)
		regLabel.TextSize = 11

		regBox := container.NewVBox(regLabel, spacerWidth2, regPattern)

		fileLabel, err := BuildFileLabel(cell, regPattern.Text)
		if err != nil {
			log.Panic(err)
		}
		fileName := container.NewStack(fileCanvas, fileLabel)

		regPattern.OnChanged = func(p string) {
			newFileLabel, err := BuildFileLabel(cell, p)
			if err == nil {
				fileName.Objects[1] = newFileLabel
				newFileLabel.Refresh()
			}
		}

		fileNameStretch := container.NewVBox(spacerWidth1, fileName)
		fileNameBox := container.NewVBox(label, fileNameStretch)

		row := container.NewHBox(margin, fileNameBox, spacer, regBox, margin)
		content.Add(row)
		content.Add(rowSpacer)
	}

	save := widget.NewButton("   Save   ", func() {
		fmt.Println("[DEBUG Saving RegEx patterns...]")
		for i, cell := range cells {
			if len(cell.Files) == 0 {
				continue
			}

			newPattern, err := regexp.Compile(strings.TrimSpace(entries[i].Text))
			if err != nil {
				fmt.Println(err)
				continue
			}

			if cell.Regex.String() != entries[i].Text {
				fmt.Printf("[DEBUG] OG Pattern: %s | New Pattern: %s\n", cell.Regex.String(), entries[i].Text)
				cell.Regex = newPattern
				RefreshFileCell(cell, i)
			}
		}
		w.Close()
	})
	saveBtn := container.NewHBox(layout.NewSpacer(), container.NewVBox(rowSpacer, save, rowSpacer), layout.NewSpacer())

	content.Add(layout.NewSpacer())

	w.SetContent(container.NewBorder(
		header,
		saveBtn,
		nil,
		nil,
		gridArea,
	))

	return w
}
