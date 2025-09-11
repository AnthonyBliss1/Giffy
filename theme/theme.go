package ui

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed JetBrains-Mono/JetBrainsMono-Regular.ttf
var jetBrains []byte

var ResourceTabular = fyne.NewStaticResource("NewRocker-Regular.ttf", jetBrains)

var Giffy = color.NRGBA{R: 0xD5, G: 0xF1, B: 0xFF, A: 0xFF}

type ForcedVariant struct {
	fyne.Theme
	Variant fyne.ThemeVariant
}

func (f *ForcedVariant) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameForeground:
		return Giffy
	case theme.ColorNamePlaceHolder:
		return Giffy
	case theme.ColorNameDisabled:
		return Giffy
	case theme.ColorNamePrimary:
		return Giffy
	}
	return f.Theme.Color(name, f.Variant)
}

func (f *ForcedVariant) Font(s fyne.TextStyle) fyne.Resource {
	return ResourceTabular
}

func (f *ForcedVariant) Icon(name fyne.ThemeIconName) fyne.Resource {
	return f.Theme.Icon(name)
}

func (f *ForcedVariant) Size(name fyne.ThemeSizeName) float32 {
	return f.Theme.Size(name)
}
