package main

import (
	"fmt"
	"image"
	"os" // Added to check and delete files
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog" // Added for the confirmation pop-up
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"Timelapse-PixelBattle/internal/db"
	"Timelapse-PixelBattle/internal/graphics"
	"Timelapse-PixelBattle/pkg/common"
	"Timelapse-PixelBattle/pkg/entities"
)

func main() {
	myApp := app.New()
	window := myApp.NewWindow("Timelapse-PB GUI Beta")
	window.Resize(fyne.NewSize(1100, 750))
	window.SetFixedSize(false)

	// For reuse
	var cachedData []entities.VisualData
	var lastCacheKey string

	// Preview canvas - dynamic?
	previewCanvas := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	previewCanvas.FillMode = canvas.ImageFillContain
	previewCanvas.SetMinSize(fyne.NewSize(250, 350))

	statusLabel := widget.NewLabel("Status: Standing By")
	statusLabel.Wrapping = fyne.TextWrapWord
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	// Input widgets
	widthEntry := widget.NewEntry()
	widthEntry.SetText("1080")
	heightEntry := widget.NewEntry()
	heightEntry.SetText("1920")
	iterationsEntry := widget.NewEntry()
	iterationsEntry.SetText("16")
	texSizeEntry := widget.NewEntry()
	texSizeEntry.SetText("16")
	fpsEntry := widget.NewEntry()
	fpsEntry.SetText("24")
	playerNameEntry := widget.NewEntry()
	dbSourceEntry := widget.NewEntry()
	dbSourceEntry.SetText("postgres")

	// Network DB widgets extracted to variables for dynamic hiding/showing
	dbIpLabel := widget.NewLabel("DB Network Address")
	dbIpEntry := widget.NewEntry()
	dbIpEntry.SetText("127.0.0.1:5432")
	dbUserLabel := widget.NewLabel("DB User")
	dbUserEntry := widget.NewEntry()
	dbUserEntry.SetText("postgres")
	dbPasswordLabel := widget.NewLabel("DB Password")
	dbPasswordEntry := widget.NewPasswordEntry()
	dbNameLabel := widget.NewLabel("DB Database Name")
	dbNameEntry := widget.NewEntry()
	dbTableEntry := widget.NewEntry()

	outputEntry := widget.NewEntry()
	outputEntry.SetText("output.mp4")
	dbTLSToggle := widget.NewCheck("Enable DB TLS", nil)
	withInfoToggle := widget.NewCheck("With additional info", nil)
	debugToggle := widget.NewCheck("Enable Debug", nil)

	// New container for easier management
	settingsContainer := container.New(layout.NewFormLayout(),
		widget.NewLabel("Video Width"), widthEntry,
		widget.NewLabel("Video Height"), heightEntry,
		widget.NewLabel("Target Framerate"), fpsEntry,
		widget.NewLabel("Batch Iterations"), iterationsEntry,
		widget.NewLabel("Texture Resolution"), texSizeEntry,
		widget.NewLabel("Player Filter"), playerNameEntry,
		widget.NewLabel("DB Engine/Path"), dbSourceEntry,
		dbIpLabel, dbIpEntry,
		dbUserLabel, dbUserEntry,
		dbPasswordLabel, dbPasswordEntry,
		dbNameLabel, dbNameEntry,
		widget.NewLabel("DB Table Name"), dbTableEntry,
		widget.NewLabel("Output"), outputEntry,
	)

	// DB Field hide
	dbSourceEntry.OnChanged = func(text string) {
		isLocal := strings.HasSuffix(text, ".db")
		if isLocal {
			dbIpLabel.Hide()
			dbIpEntry.Hide()
			dbUserLabel.Hide()
			dbUserEntry.Hide()
			dbPasswordLabel.Hide()
			dbPasswordEntry.Hide()
			dbNameLabel.Hide()
			dbNameEntry.Hide()
		} else {
			dbIpLabel.Show()
			dbIpEntry.Show()
			dbUserLabel.Show()
			dbUserEntry.Show()
			dbPasswordLabel.Show()
			dbPasswordEntry.Show()
			dbNameLabel.Show()
			dbNameEntry.Show()
		}
		settingsContainer.Refresh()
	}

	// GPU Getters
	detectedGPUs := common.GetAvailableGPUs()
	gpuOptionStrings := []string{"Automated Selection", "Software Encoder (libx264)"}
	for _, g := range detectedGPUs {
		typeStr := "Discrete"
		if g.IsIntegrated {
			typeStr = "Integrated"
		}
		gpuOptionStrings = append(gpuOptionStrings, fmt.Sprintf("%s (%s - %s)", g.Name, g.Vendor, typeStr))
	}
	selectedHardwareKey := gpuOptionStrings[0]
	gpuSelector := widget.NewSelect(gpuOptionStrings, func(choice string) {
		selectedHardwareKey = choice
	})
	gpuSelector.SetSelected(selectedHardwareKey)
	resolveSelectedHardware := func(w, h int) entities.GPUSelection {
		if selectedHardwareKey == "Software Encoder (libx264)" {
			return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
		}
		for _, g := range detectedGPUs {
			expectedLabel := fmt.Sprintf("%s (%s - %s)", g.Name, g.Vendor, "Discrete")
			if g.IsIntegrated {
				expectedLabel = fmt.Sprintf("%s (%s - %s)", g.Name, g.Vendor, "Integrated")
			}
			if selectedHardwareKey == expectedLabel {
				return graphics.ResolveEncoderForGPU(g)
			}
		}
		return entities.GPUSelection{Encoder: "libx264", EncoderName: "libx264", GPUType: "cpu"}
	}

	// Main loop
	runTask := func(commandType string) {
		wStr := widthEntry.Text
		hStr := heightEntry.Text
		iterStr := iterationsEntry.Text
		texStr := texSizeEntry.Text
		fpsStr := fpsEntry.Text
		pName := playerNameEntry.Text
		dbSrc := dbSourceEntry.Text
		dbIp := dbIpEntry.Text
		dbUser := dbUserEntry.Text
		dbPass := dbPasswordEntry.Text
		dbName := dbNameEntry.Text
		dbTable := dbTableEntry.Text
		outPath := outputEntry.Text
		tlsCheck := dbTLSToggle.Checked
		infoCheck := withInfoToggle.Checked
		dbgCheck := debugToggle.Checked

		localCheck := strings.HasSuffix(dbSrc, ".db")

		proceedWithTask := func() {
			go func() {
				w, _ := strconv.Atoi(wStr)
				h, _ := strconv.Atoi(hStr)
				iterations, _ := strconv.Atoi(iterStr)
				texSize, _ := strconv.Atoi(texStr)
				fps, _ := strconv.Atoi(fpsStr)

				if w <= 0 || h <= 0 {
					fyne.Do(func() { statusLabel.SetText("Status: Error! Resolution dimensions must be greater than 0.") })
					return
				}
				previewW := 360
				previewH := int(float64(h) * (float64(previewW) / float64(w)))
				previewBuffer := image.NewRGBA(image.Rect(0, 0, previewW, previewH))

				fyne.Do(func() {
					previewCanvas.Image = previewBuffer
					previewCanvas.Refresh()
				})

				hw := resolveSelectedHardware(w, h)

				cli := entities.CLI{
					Width: w, Height: h, Iterations: iterations, TextureSize: texSize, Framerate: fps,
					PlayerName: pName, DBSource: dbSrc, DBIp: dbIp,
					DBUser: dbUser, DBPassword: dbPass, DBName: dbName,
					DBTable: dbTable, DBTLS: tlsCheck, Local: localCheck,
					WithInfo: infoCheck, Debug: dbgCheck,
				}

				currentCacheKey := fmt.Sprintf("%s|%s|%s|%s|%s|%v",
					cli.DBSource, cli.DBIp, cli.DBName, cli.DBTable, cli.PlayerName, localCheck)

				if err := graphics.LoadTextureAtlas("assets", cli.TextureSize); err != nil {
					fyne.Do(func() { statusLabel.SetText(fmt.Sprintf("Texture Error: %v", err)) })
					return
				}
				globalTimer := time.Now()

				if lastCacheKey == currentCacheKey && len(cachedData) > 0 {
					fyne.Do(func() {
						statusLabel.SetText("Status: Hot Memory Hit: Reusing data layer from local allocation cache.")
					})
				} else {
					fyne.Do(func() {
						statusLabel.SetText("Status: Initializing active database query pipes...")
						progressBar.SetValue(0)
						progressBar.Show()
					})

					db.Init(cli.DBSource, cli.DBIp, cli.DBUser, cli.DBPassword, cli.DBName, cli.DBTLS, cli.Local)
					num, _ := db.GetMaxCount(cli.DBTable, cli.PlayerName)

					cachedData = make([]entities.VisualData, 0, num)
					var lastID int64

					for {
						sub := db.GetData(cli.PlayerName, cli.DBTable, lastID)
						if sub == nil || len(*sub) == 0 {
							break
						}
						cachedData = append(cachedData, *sub...)
						lastItem := (*sub)[len(*sub)-1]
						lastID = lastItem.Id

						currentProgress := len(cachedData)
						fyne.Do(func() {
							statusLabel.SetText(fmt.Sprintf("Status: Ingesting records: %d / %d", currentProgress, num))
							if num > 0 {
								progressBar.SetValue(float64(currentProgress) / float64(num))
							}
						})
					}
					db.Close()
					lastCacheKey = currentCacheKey
				}

				var taskErr error
				fyne.Do(func() {
					progressBar.SetValue(0)
					progressBar.Hide()
				})
				switch commandType {
				case "render":
					fyne.Do(func() {
						statusLabel.SetText(fmt.Sprintf("Status: Rendering video via encoder: %s (%s)", hw.EncoderName, hw.Encoder))
						progressBar.SetValue(0)
						progressBar.Show()
					})
					taskErr = graphics.EncodeGPU(cachedData, cli.Width, cli.Height, cli.Iterations, cli.TextureSize, cli.Framerate,
						outPath, cli.PlayerName, hw, cli.WithInfo, cli.Debug, previewBuffer,
						func(progress float64) {
							fyne.Do(func() {
								progressBar.SetValue(progress)
								previewCanvas.Refresh()
							})
						})
				case "photo":
					fyne.Do(func() { statusLabel.SetText("Status: Processing spatial image array blitting...") })

					var photoImg image.Image
					photoImg, taskErr = graphics.GeneratePhotoLocal(&cachedData, cli.Width, cli.Height, cli.TextureSize, outPath)

					if taskErr == nil && photoImg != nil {
						fyne.Do(func() {
							previewCanvas.Image = photoImg
							previewCanvas.Refresh()
						})
					}
				}

				fyne.Do(func() {
					progressBar.SetValue(0)
					progressBar.Hide()
					if taskErr != nil {
						statusLabel.SetText(fmt.Sprintf("Status: Pipeline failed with reason: %v", taskErr))
					} else {
						statusLabel.SetText(fmt.Sprintf("Status: Task complete! Processing duration: %v", time.Since(globalTimer).Round(time.Millisecond)))
					}
				})
			}()
		}

		if _, err := os.Stat(outPath); err == nil {
			dialog.ShowConfirm(
				"Overwrite Existing File?",
				fmt.Sprintf("The output destination file '%s' already exists.\nDo you want to permanently delete and overwrite it?", outPath),
				func(confirm bool) {
					if confirm {
						// checking for files.
						lowerPath := strings.ToLower(outPath)

						// Safety from user input (idc, they CAN and WILL SPECIFY SOME OS FILES AND APP MUST PREVENT THIS)
						isProtectedSystemPath := outPath == "" || outPath == "/" || outPath == "\\" ||
							strings.HasPrefix(lowerPath, "/etc") ||
							strings.HasPrefix(lowerPath, "/bin") ||
							strings.HasPrefix(lowerPath, "/sys") ||
							strings.HasPrefix(lowerPath, "/usr") ||
							strings.HasPrefix(lowerPath, "/var") ||
							strings.HasPrefix(lowerPath, "c:\\windows") ||
							strings.HasPrefix(lowerPath, "c:\\program files")
						if isProtectedSystemPath {
							statusLabel.SetText(fmt.Sprintf("Status: Safety Intercept! Refusing to delete potentially hazardous or non-media path: %s", outPath))
							return
						}

						// Explicitly delete the old file safely if it passes all constraints
						if err = os.Remove(outPath); err != nil {
							statusLabel.SetText(fmt.Sprintf("Status: Failed to remove old file: %v", err))
						}
						// After all of THAT, run.
						proceedWithTask()
					} else {
						statusLabel.SetText("Status: Aborted. Output file preserve constraint triggered.")
					}
				},
				window,
			)
		} else {
			proceedWithTask()
		}
	}

	// Sections
	createSection := func(title string, content fyne.CanvasObject, startOpen bool) fyne.CanvasObject {
		content.Refresh()
		if !startOpen {
			content.Hide()
		}

		var toggleBtn *widget.Button
		getIconStr := func(visible bool) string {
			if visible {
				return " -   " + title
			}
			return " +  " + title
		}

		toggleBtn = widget.NewButton(getIconStr(startOpen), func() {
			if content.Visible() {
				content.Hide()
				toggleBtn.SetText(getIconStr(false))
			} else {
				content.Show()
				toggleBtn.SetText(getIconStr(true))
			}
			content.Refresh()
		})
		toggleBtn.Importance = widget.LowImportance
		toggleBtn.Alignment = widget.ButtonAlignLeading

		return container.NewVBox(toggleBtn, content)
	}

	renderBtn := widget.NewButton("Render video", func() { runTask("render") })
	photoBtn := widget.NewButton("Create photo", func() { runTask("photo") })

	togglesContainer := container.NewGridWithColumns(2, dbTLSToggle, withInfoToggle, debugToggle)
	rendererSidebar := container.NewVScroll(container.NewVBox(
		createSection("Configuration", settingsContainer, true),
		widget.NewSeparator(),
		createSection("HW Driver", gpuSelector, true),
		widget.NewSeparator(),
		createSection("Additional configuration", togglesContainer, false),
		widget.NewSeparator(),
		container.NewGridWithColumns(2, renderBtn, photoBtn),
	))
	makerSidebar := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("TL-Generator", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("W.I.P."),
	))

	sidebarStack := container.NewStack(rendererSidebar)
	rendererSidebar.SetMinSize(fyne.NewSize(350, 0))
	makerSidebar.SetMinSize(fyne.NewSize(350, 0))

	var btnRenderer *widget.Button
	var btnMaker *widget.Button

	switchMode := func(targetMode string) {
		if targetMode == "Renderer" {
			sidebarStack.Objects = []fyne.CanvasObject{rendererSidebar}
			rendererSidebar.Show()
			makerSidebar.Hide()
			btnRenderer.Importance = widget.HighImportance
			btnMaker.Importance = widget.LowImportance
		} else {
			sidebarStack.Objects = []fyne.CanvasObject{makerSidebar}
			makerSidebar.Show()
			rendererSidebar.Hide()
			btnRenderer.Importance = widget.LowImportance
			btnMaker.Importance = widget.HighImportance
		}
		btnRenderer.Refresh()
		btnMaker.Refresh()
		sidebarStack.Refresh()
	}

	btnRenderer = widget.NewButton("TL-Render", func() { switchMode("Renderer") })
	btnMaker = widget.NewButton("TL-Generator", func() { switchMode("Maker") })
	btnRenderer.Importance = widget.HighImportance
	btnMaker.Importance = widget.LowImportance

	segmentedTabs := container.NewGridWithColumns(2, btnRenderer, btnMaker)
	topModeHeader := container.NewBorder(
		nil,
		widget.NewSeparator(),
		nil, nil,
		container.NewHBox(
			widget.NewLabel("Workspace Mode:"),
			segmentedTabs,
		),
	)

	headerWithBorder := container.NewBorder(
		nil,
		widget.NewSeparator(),
		nil, nil,
		widget.NewLabelWithStyle("Output view", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)
	rightDisplayLayout := container.NewBorder(
		headerWithBorder,
		container.NewVBox(statusLabel, progressBar),
		nil, nil,
		previewCanvas,
	)

	sidebarWithDivider := container.NewBorder(nil, nil, nil, widget.NewSeparator(), sidebarStack)
	staticLayout := container.NewBorder(topModeHeader, nil, sidebarWithDivider, nil, rightDisplayLayout)

	window.SetContent(staticLayout)
	window.ShowAndRun()
}
