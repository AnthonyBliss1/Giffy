package main

import (
	_ "embed"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	windows "github.com/anthonybliss1/giffy/gui/windows"
	ui "github.com/anthonybliss1/giffy/theme"
)

func main() {
	a := app.New()

	base := theme.DefaultTheme()
	a.Settings().SetTheme(&ui.ForcedVariant{
		Theme:   base,
		Variant: theme.VariantDark,
	})

	a.SetIcon(windows.IconLogo)

	config := windows.ConfigWindow(a)
	config.CenterOnScreen()
	config.ShowAndRun()
}
