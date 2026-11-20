package main

import (
	"context"

	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// imagePreview is a picture the pane shows, decoded from its file.
type imagePreview struct {
	src     fileSource
	path    string
	name    string
	pic     ui.ImageSource
	width   float32
	height  float32
	loading bool
	err     string
}

// openImage shows a picture in a tab of the pane, or the tab it has.
func (w *Window) openImage(src fileSource, file, name string) {
	if img := w.paneImage(src, file); img != nil {
		for i, tab := range w.paneTabs {
			if tab.image == img {
				w.paneSel = i
				break
			}
		}
		w.syncMenus()
		w.invalidate()
		return
	}
	img := &imagePreview{src: src, path: file, name: name, loading: true}
	w.paneTabs = append(w.paneTabs, &paneTab{image: img})
	w.paneSel = len(w.paneTabs) - 1
	w.syncMenus()
	w.loadImage(img)
}

// paneImage is the picture the pane holds, or nil.
func (w *Window) paneImage(src fileSource, file string) *imagePreview {
	for _, t := range w.paneTabs {
		if t.image != nil && t.image.src == src && t.image.path == file {
			return t.image
		}
	}
	return nil
}

// holdsImage reports whether the pane still holds the picture.
func (w *Window) holdsImage(img *imagePreview) bool {
	for _, t := range w.paneTabs {
		if t.image == img {
			return true
		}
	}
	return false
}

// loadImage decodes a file's picture, in the background.
func (w *Window) loadImage(img *imagePreview) {
	img.loading, img.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
		defer cancel()
		data, err := readFile(ctx, img.src, img.path)
		if err != nil {
			w.imageFailed(img, err.Error())
			return
		}
		pic, err := ui.DecodeImage(data)
		if err != nil {
			w.imageFailed(img, err.Error())
			return
		}
		width, height := ui.ImageSize(pic)
		w.onMain(func() {
			if !w.holdsImage(img) {
				return
			}
			img.pic, img.width, img.height, img.loading = pic, width, height, false
			w.invalidate()
		})
	}()
}

// imageFailed shows what went wrong with a picture, in its tab.
func (w *Window) imageFailed(img *imagePreview, text string) {
	w.onMain(func() {
		if !w.holdsImage(img) {
			return
		}
		img.loading = false
		img.err = text
		w.invalidate()
	})
}

// imageBody shows a picture, as large as its tab allows it.
func (w *Window) imageBody(c *ui.Context, lk *look, img *imagePreview) {
	switch {
	case img.loading:
		ui.Column(c).FillWidth().Grow(1).Center().Children(func() {
			ui.Spinner(c).Label(locales.FilesLoading())
		})
	case img.err != "":
		ui.Column(c).FillWidth().Grow(1).Center().Padding(16).Gap(8).Children(func() {
			ui.Text(c, img.err).FontSize(12).TextColor(lk.danger).TextAlign(ui.Center).
				LineHeight(1.35).MaxWidth(320)
			if ui.Button(c, locales.FilesRetry()).Clicked() {
				w.loadImage(img)
			}
		})
	default:
		ui.Column(c).FillWidth().Grow(1).Padding(12).Children(func() {
			ui.Image(c, img.pic).FillWidth().Grow(1).Fit(ui.ScaleDown).Label(img.name)
		})
	}
}

// imageButtons are the picture's own actions: reading it again, and the
// app the system has for it.
func (w *Window) imageButtons(c *ui.Context, lk *look, img *imagePreview) {
	w.rowButton(c, lk, iconRefresh, locales.FilesRefresh(), func() { w.loadImage(img) })
	w.rowButton(c, lk, iconExternal, locales.EditorOpenExternal(), func() {
		w.openExternally(img.src, img.path)
	})
}
