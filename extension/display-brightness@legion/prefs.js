import Adw from 'gi://Adw';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import Gtk from 'gi://Gtk';
import {ExtensionPreferences} from 'resource:///org/gnome/Shell/Extensions/js/extensions/prefs.js';

const BUS_NAME = 'org.display.Brightness';
const OBJECT_PATH = '/org/display/Brightness';
const INTERFACE = 'org.display.Brightness';

function call(method, params, replyType) {
    return new Promise((resolve, reject) => {
        Gio.DBus.session.call(
            BUS_NAME,
            OBJECT_PATH,
            INTERFACE,
            method,
            params,
            replyType ? new GLib.VariantType(replyType) : null,
            Gio.DBusCallFlags.NONE,
            -1,
            null,
            (conn, result) => {
                try {
                    const variant = conn.call_finish(result);
                    resolve(replyType ? variant.deepUnpack() : null);
                } catch (e) {
                    reject(e);
                }
            }
        );
    });
}

const CurveView = GObject.registerClass(
class CurveView extends Gtk.DrawingArea {
    _init() {
        super._init({
            hexpand: true,
            vexpand: true,
            content_width: 480,
            content_height: 280,
            can_focus: true,
        });
        this._points = [
            [0, 10],
            [50, 40],
            [200, 70],
            [1000, 100],
        ];
        this._lux = null;
        this._dragIndex = -1;
        this._maxLux = 1000;

        this.set_draw_func((_area, cr, width, height) => this._draw(cr, width, height));

        const drag = new Gtk.GestureDrag();
        drag.connect('drag-begin', (_g, x, y) => this._onDragBegin(x, y));
        drag.connect('drag-update', (_g, dx, dy) => this._onDragUpdate(dx, dy));
        drag.connect('drag-end', () => {
            this._dragIndex = -1;
        });
        this.add_controller(drag);

        const click = new Gtk.GestureClick();
        click.connect('pressed', (_g, n, x, y) => {
            if (n === 1 && this._hitPoint(x, y) < 0)
                this._addPointAt(x, y);
        });
        this.add_controller(click);

        const right = new Gtk.GestureClick({button: 3});
        right.connect('pressed', (_g, _n, x, y) => {
            const i = this._hitPoint(x, y);
            if (i > 0 && i < this._points.length - 1) {
                this._points.splice(i, 1);
                this.queue_draw();
            }
        });
        this.add_controller(right);
    }

    setPoints(points) {
        this._points = points.map(p => [p[0], p[1]]);
        this._maxLux = Math.max(100, ...this._points.map(p => p[0]));
        this.queue_draw();
    }

    getPoints() {
        return this._points.map(([lux, bri]) => [Math.round(lux), Math.round(bri)]);
    }

    setLux(lux) {
        this._lux = lux;
        this.queue_draw();
    }

    _pad() {
        return {l: 48, r: 16, t: 16, b: 36};
    }

    _toScreen(lux, bri, width, height) {
        const p = this._pad();
        const x = p.l + (lux / this._maxLux) * (width - p.l - p.r);
        const y = p.t + (1 - bri / 100) * (height - p.t - p.b);
        return [x, y];
    }

    _fromScreen(x, y, width, height) {
        const p = this._pad();
        const lux = ((x - p.l) / Math.max(1, width - p.l - p.r)) * this._maxLux;
        const bri = (1 - (y - p.t) / Math.max(1, height - p.t - p.b)) * 100;
        return [
            Math.max(0, Math.min(this._maxLux, lux)),
            Math.max(0, Math.min(100, bri)),
        ];
    }

    _hitPoint(x, y) {
        const w = this.get_width();
        const h = this.get_height();
        for (let i = 0; i < this._points.length; i++) {
            const [sx, sy] = this._toScreen(this._points[i][0], this._points[i][1], w, h);
            if ((sx - x) ** 2 + (sy - y) ** 2 <= 100)
                return i;
        }
        return -1;
    }

    _onDragBegin(x, y) {
        this._dragIndex = this._hitPoint(x, y);
        this._dragOrigin = [x, y];
    }

    _onDragUpdate(dx, dy) {
        if (this._dragIndex < 0)
            return;
        const w = this.get_width();
        const h = this.get_height();
        const [x0, y0] = this._dragOrigin;
        let [lux, bri] = this._fromScreen(x0 + dx, y0 + dy, w, h);
        const i = this._dragIndex;
        if (i === 0)
            lux = 0;
        else if (i === this._points.length - 1)
            lux = this._maxLux;
        else {
            const minLux = this._points[i - 1][0] + 1;
            const maxLux = this._points[i + 1][0] - 1;
            lux = Math.max(minLux, Math.min(maxLux, lux));
        }
        this._points[i] = [lux, bri];
        this.queue_draw();
    }

    _addPointAt(x, y) {
        const w = this.get_width();
        const h = this.get_height();
        const [lux, bri] = this._fromScreen(x, y, w, h);
        if (lux <= 0 || lux >= this._maxLux)
            return;
        this._points.push([lux, bri]);
        this._points.sort((a, b) => a[0] - b[0]);
        this.queue_draw();
    }

    _draw(cr, width, height) {
        const p = this._pad();
        const plotW = width - p.l - p.r;
        const plotH = height - p.t - p.b;

        cr.setSourceRGB(0.12, 0.12, 0.14);
        cr.rectangle(0, 0, width, height);
        cr.fill();

        cr.setSourceRGB(0.22, 0.22, 0.26);
        cr.rectangle(p.l, p.t, plotW, plotH);
        cr.fill();

        cr.setSourceRGB(0.35, 0.35, 0.4);
        cr.setLineWidth(1);
        for (let i = 0; i <= 4; i++) {
            const y = p.t + (plotH * i) / 4;
            cr.moveTo(p.l, y);
            cr.lineTo(p.l + plotW, y);
            cr.stroke();
        }

        if (this._points.length >= 2) {
            cr.setSourceRGB(0.35, 0.7, 1.0);
            cr.setLineWidth(2);
            const [x0, y0] = this._toScreen(this._points[0][0], this._points[0][1], width, height);
            cr.moveTo(x0, y0);
            for (let i = 1; i < this._points.length; i++) {
                const [x, y] = this._toScreen(this._points[i][0], this._points[i][1], width, height);
                cr.lineTo(x, y);
            }
            cr.stroke();
        }

        for (const [lux, bri] of this._points) {
            const [x, y] = this._toScreen(lux, bri, width, height);
            cr.setSourceRGB(1, 1, 1);
            cr.arc(x, y, 6, 0, Math.PI * 2);
            cr.fill();
            cr.setSourceRGB(0.2, 0.55, 0.9);
            cr.arc(x, y, 4, 0, Math.PI * 2);
            cr.fill();
        }

        if (this._lux !== null && this._maxLux > 0) {
            const [x] = this._toScreen(Math.min(this._lux, this._maxLux), 0, width, height);
            cr.setSourceRGBA(1, 0.8, 0.2, 0.8);
            cr.setLineWidth(1.5);
            cr.moveTo(x, p.t);
            cr.lineTo(x, p.t + plotH);
            cr.stroke();
        }

        cr.setSourceRGB(0.75, 0.75, 0.8);
        cr.selectFontFace('Sans', 0, 0);
        cr.setFontSize(11);
        cr.moveTo(8, p.t + 12);
        cr.showText('100%');
        cr.moveTo(8, p.t + plotH);
        cr.showText('0%');
        cr.moveTo(p.l, height - 12);
        cr.showText('0 lux');
        cr.moveTo(width - 70, height - 12);
        cr.showText(`${this._maxLux} lux`);
    }
});

export default class DisplayBrightnessPrefs extends ExtensionPreferences {
    fillPreferencesWindow(window) {
        window.set_default_size(560, 480);
        window.title = 'Lighting curve';

        const page = new Adw.PreferencesPage();
        const group = new Adw.PreferencesGroup({
            title: 'Lux → brightness',
            description: 'Drag points to reshape the curve. Click empty space to add a point; right-click a middle point to remove.',
        });

        const curve = new CurveView();
        const luxLabel = new Gtk.Label({
            label: 'Lux: —',
            xalign: 0,
            margin_top: 8,
        });
        const status = new Gtk.Label({
            label: '',
            xalign: 0,
            margin_top: 4,
        });

        const saveBtn = new Gtk.Button({
            label: 'Save curve',
            css_classes: ['suggested-action'],
            halign: Gtk.Align.START,
            margin_top: 12,
        });
        saveBtn.connect('clicked', async () => {
            const pts = curve.getPoints();
            const variant = new GLib.Variant('(a(uu))', [pts.map(([l, b]) => [l, b])]);
            try {
                await call('SetCurve', variant, null);
                status.label = 'Saved.';
            } catch (e) {
                status.label = `Save failed: ${e.message}`;
            }
        });

        const box = new Gtk.Box({
            orientation: Gtk.Orientation.VERTICAL,
            spacing: 4,
            margin_top: 8,
            margin_bottom: 8,
        });
        box.append(curve);
        box.append(luxLabel);
        box.append(saveBtn);
        box.append(status);
        group.add(box);
        page.add(group);
        window.add(page);

        const load = async () => {
            try {
                const [points] = await call('GetCurve', null, '(a(uu))');
                curve.setPoints(points);
                const [lux] = await call('GetLux', null, '(u)');
                curve.setLux(lux);
                luxLabel.label = `Lux: ${lux}`;
            } catch (e) {
                status.label = `Could not load from daemon: ${e.message}`;
            }
        };
        load();

        this._luxSub = Gio.DBus.session.signal_subscribe(
            BUS_NAME,
            INTERFACE,
            'LuxChanged',
            OBJECT_PATH,
            null,
            Gio.DBusSignalFlags.NONE,
            (_c, _s, _p, _i, _sig, params) => {
                const [lux] = params.deepUnpack();
                curve.setLux(lux);
                luxLabel.label = `Lux: ${lux}`;
            }
        );

        window.connect('close-request', () => {
            if (this._luxSub) {
                Gio.DBus.session.signal_unsubscribe(this._luxSub);
                this._luxSub = null;
            }
            return false;
        });
    }
}
