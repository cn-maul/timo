//go:build linux

package app

/*
#cgo pkg-config: gtk4 x11
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <string.h>

// configure_window_x11 sets SKIP_TASKBAR, SKIP_PAGER, KEEP_ABOVE and
// _NET_WM_WINDOW_TYPE_DOCK via X11 ClientMessage and direct property changes.
// GTK4 removed GdkWindowTypeHint, so we do this at the X11 level.
// Only works on X11; silently no-ops on Wayland.
static void configure_window_x11(GtkWindow *win) {
    GtkWidget *widget = GTK_WIDGET(win);
    GtkNative *native = gtk_widget_get_native(widget);
    if (!native) return;

    GdkSurface *surface = gtk_native_get_surface(native);
    if (!surface) return;

    GdkDisplay *display = gdk_surface_get_display(surface);
    if (!GDK_IS_X11_DISPLAY(display)) return;

    Display *xdisplay = gdk_x11_display_get_xdisplay(display);
    Window xwindow = gdk_x11_surface_get_xid(GDK_X11_SURFACE(surface));
    Atom wm_state = XInternAtom(xdisplay, "_NET_WM_STATE", False);

    // --- 1. Set _NET_WM_WINDOW_TYPE to DOCK ---
    // This is the most reliable way to get skip-taskbar behavior.
    // Window managers treat DOCK windows as panels/bars.
    Atom type_atom   = XInternAtom(xdisplay, "_NET_WM_WINDOW_TYPE", False);
    Atom type_dock   = XInternAtom(xdisplay, "_NET_WM_WINDOW_TYPE_DOCK", False);
    XChangeProperty(xdisplay, xwindow, type_atom, XA_ATOM, 32,
                    PropModeReplace, (unsigned char *)&type_dock, 1);

    // --- 2. Request SKIP_TASKBAR + SKIP_PAGER + KEEP_ABOVE ---
    Atom skip_taskbar = XInternAtom(xdisplay, "_NET_WM_STATE_SKIP_TASKBAR", False);
    Atom skip_pager   = XInternAtom(xdisplay, "_NET_WM_STATE_SKIP_PAGER", False);
    Atom above        = XInternAtom(xdisplay, "_NET_WM_STATE_ABOVE", False);

    // Use ClientMessage to request WM add SKIP_TASKBAR + SKIP_PAGER
    XEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.xclient.type = ClientMessage;
    ev.xclient.window = xwindow;
    ev.xclient.message_type = wm_state;
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
    ev.xclient.data.l[1] = skip_taskbar;
    ev.xclient.data.l[2] = skip_pager;
    ev.xclient.data.l[3] = 1; // normal source

    XSendEvent(xdisplay, DefaultRootWindow(xdisplay), False,
               SubstructureRedirectMask | SubstructureNotifyMask, &ev);

    // Also add KEEP_ABOVE (ClientMessage supports max 2 atoms per message)
    memset(&ev, 0, sizeof(ev));
    ev.xclient.type = ClientMessage;
    ev.xclient.window = xwindow;
    ev.xclient.message_type = wm_state;
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
    ev.xclient.data.l[1] = above;
    ev.xclient.data.l[3] = 1;

    XSendEvent(xdisplay, DefaultRootWindow(xdisplay), False,
               SubstructureRedirectMask | SubstructureNotifyMask, &ev);
    XFlush(xdisplay);
}
#pragma GCC diagnostic pop
*/
import "C"

import (
	"log"
	"unsafe"
)

// configureDockWindow sets up the window as a dock/panel that:
// - Does not appear in the taskbar (SKIP_TASKBAR + _NET_WM_WINDOW_TYPE_DOCK)
// - Does not appear in pager/Alt-Tab (SKIP_PAGER)
// - Stays above all normal windows (KEEP_ABOVE)
//
// Uses X11 protocol directly since GTK4 removed GdkWindowTypeHint.
// On Wayland this is a no-op (no standard protocol for skip-taskbar).
//
// Call this after the window is shown (e.g. in the WindowShow event handler)
// so the GTK surface is realized and X11 properties can be applied.
func configureDockWindow(nativePtr unsafe.Pointer) {
	if nativePtr == nil {
		return
	}
	gtkWin := (*C.GtkWindow)(nativePtr)
	C.configure_window_x11(gtkWin)
	log.Println("timo: configured as dock window (skip taskbar, always on top)")
}
