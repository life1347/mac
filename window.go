package mac

/*
#include "window.h"
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"strconv"
	"unsafe"

	"math"

	"github.com/murlokswarm/app"
	"github.com/murlokswarm/errors"
	"github.com/murlokswarm/log"
	"github.com/murlokswarm/markup"
	"github.com/murlokswarm/uid"
)

var (
	winPtrChan    = make(chan unsafe.Pointer)
	winloadedChan = make(chan bool)
)

type window struct {
	id        uid.ID
	ptr       unsafe.Pointer
	component app.Componer
	config    app.Window
}

func newWindow(w app.Window) *window {
	id := uid.Context()
	css, err := app.GetFilenamesWithExtensionsFromDir(app.Storage().CSS(), ".css")
	if err != nil {
		log.Warn(err)
	}
	js, err := app.GetFilenamesWithExtensionsFromDir(app.Storage().JS(), ".js")
	if err != nil {
		log.Warn(err)
	}
	htmlCtx := app.HTMLContext{
		ID:       id,
		Title:    w.Title,
		Lang:     w.Lang,
		MurlokJS: app.MurlokJS(),
		JS:       js,
		CSS:      css,
	}
	if w.MaxWidth <= 0 {
		w.MaxWidth = 10000
	}
	if w.MaxHeight <= 0 {
		w.MaxHeight = 10000
	}
	cwin := C.Window__{
		ID:              C.CString(id.String()),
		Title:           C.CString(w.Title),
		X:               C.CGFloat(w.X),
		Y:               C.CGFloat(w.Y),
		Width:           C.CGFloat(w.Width),
		Height:          C.CGFloat(w.Height),
		MinWidth:        C.CGFloat(math.Max(0, w.MinWidth)),
		MinHeight:       C.CGFloat(math.Max(0, w.MinHeight)),
		MaxWidth:        C.CGFloat(math.Min(w.MaxWidth, 10000)),
		MaxHeight:       C.CGFloat(math.Min(w.MaxHeight, 10000)),
		BackgroundColor: C.CString(w.BackgroundColor),
		Vibrancy:        C.NSVisualEffectMaterial(w.Vibrancy),
		Borderless:      boolToBOOL(w.Borderless),
		FixedSize:       boolToBOOL(w.FixedSize),
		CloseHidden:     boolToBOOL(w.CloseHidden),
		MinimizeHidden:  boolToBOOL(w.MinimizeHidden),
		TitlebarHidden:  boolToBOOL(w.TitlebarHidden),
		HTML:            C.CString(htmlCtx.HTML()),
		ResourcePath:    C.CString(app.Storage().Resources()),
	}
	defer free(unsafe.Pointer(cwin.ID))
	defer free(unsafe.Pointer(cwin.Title))
	defer free(unsafe.Pointer(cwin.BackgroundColor))
	defer free(unsafe.Pointer(cwin.HTML))
	defer free(unsafe.Pointer(cwin.ResourcePath))

	C.Window_New(cwin)
	ptr := <-winPtrChan
	<-winloadedChan

	win := &window{
		id:     id,
		ptr:    ptr,
		config: w,
	}
	app.RegisterContext(win)

	C.Window_Show(win.ptr)
	return win
}

func (w *window) ID() uid.ID {
	return w.id
}

func (w *window) Mount(c app.Componer) {
	if w.component != nil {
		markup.Dismount(w.component)
	}

	w.component = c
	if _, err := markup.Mount(c, w.ID()); err != nil {
		log.Panic(err)
	}

	html := markup.Markup(c)
	html = strconv.Quote(html)
	call := fmt.Sprintf(`Mount("%v", %v)`, w.ID(), html)
	ccall := C.CString(call)
	defer free(unsafe.Pointer(ccall))

	C.Window_CallJS(w.ptr, ccall)
}

func (w *window) Render(s markup.Sync) {
	if s.Scope == markup.FullSync {
		w.renderFullNode(s.Node)
		return
	}
	w.renderAttributes(s.Node.ID, s.Attributes)
}

func (w *window) renderFullNode(n *markup.Node) {
	html := strconv.Quote(n.Markup())
	call := fmt.Sprintf(`RenderFull("%v", %v)`, n.ID, html)
	ccall := C.CString(call)
	defer free(unsafe.Pointer(ccall))

	C.Window_CallJS(w.ptr, ccall)
}

func (w *window) renderAttributes(nodeID uid.ID, attrs markup.AttributeMap) {
	d, err := json.Marshal(attrs)
	if err != nil {
		log.Panic(errors.New(err))
	}

	call := fmt.Sprintf(`RenderAttributes("%v", %v)`, nodeID, string(d))
	ccall := C.CString(call)
	defer free(unsafe.Pointer(ccall))

	C.Window_CallJS(w.ptr, ccall)
}

func (w *window) Position() (x float64, y float64) {
	frame := C.Window_Frame(w.ptr)
	x = float64(frame.origin.x)
	y = float64(frame.origin.y)
	return
}

func (w *window) Move(x float64, y float64) {
	C.Window_Move(w.ptr, C.CGFloat(x), C.CGFloat(y))
}

func (w *window) Size() (width float64, height float64) {
	frame := C.Window_Frame(w.ptr)
	width = float64(frame.size.width)
	height = float64(frame.size.height)
	return
}

func (w *window) Resize(width float64, height float64) {
	C.Window_Resize(w.ptr, C.CGFloat(width), C.CGFloat(height))
}

func (w *window) Close() {
	C.Window_Close(w.ptr)
}

//export onWindowCreated
func onWindowCreated(ptr unsafe.Pointer) {
	winPtrChan <- ptr
}

//export onWindowWebviewLoaded
func onWindowWebviewLoaded() {
	winloadedChan <- true
}

//export onWindowMinimize
func onWindowMinimize(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnMinimize != nil {
			win.config.OnMinimize()
		}
	}
}

//export onWindowDeminimize
func onWindowDeminimize(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnDeminimize != nil {
			win.config.OnDeminimize()
		}
	}
}

//export onWindowFullScreen
func onWindowFullScreen(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnFullScreen != nil {
			win.config.OnFullScreen()
		}
	}
}

//export onWindowExitFullScreen
func onWindowExitFullScreen(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnExitFullScreen != nil {
			win.config.OnExitFullScreen()
		}
	}
}

//export onWindowMove
func onWindowMove(cid *C.char, cx C.CGFloat, cy C.CGFloat) {
	id := C.GoString(cid)
	x := float64(cx)
	y := float64(cy)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnMove != nil {
			win.config.OnMove(x, y)
		}
	}
}

//export onWindowResize
func onWindowResize(cid *C.char, width C.CGFloat, height C.CGFloat) {
	id := C.GoString(cid)
	w := float64(width)
	h := float64(height)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnResize != nil {
			win.config.OnResize(w, h)
		}
	}
}

//export onWindowFocus
func onWindowFocus(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnFocus != nil {
			win.config.OnFocus()
		}
	}
}

//export onWindowBlur
func onWindowBlur(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		if win.config.OnBlur != nil {
			win.config.OnBlur()
		}
	}
}

//export onWindowClose
func onWindowClose(cid *C.char) bool {
	id := C.GoString(cid)
	closeChan := make(chan bool)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			closeChan <- true
			return
		}
		win := ctx.(*window)
		if win.config.OnClose != nil {
			closeChan <- win.config.OnClose()
			return
		}
		closeChan <- true
	}

	return <-closeChan
}

//export onWindowCloseFinal
func onWindowCloseFinal(cid *C.char) {
	id := C.GoString(cid)

	app.UIChan <- func() {
		ctx, err := app.ContextByID(uid.ID(id))
		if err != nil {
			return
		}
		win := ctx.(*window)
		markup.Dismount(win.component)
		app.UnregisterContext(win)
	}
}
