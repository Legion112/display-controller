import Adw from 'gi://Adw';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import Gtk from 'gi://Gtk';
import {ExtensionPreferences} from 'resource:///org/gnome/Shell/Extensions/js/extensions/prefs.js';

const BUS_NAME = 'org.display.Brightness';
const OBJECT_PATH = '/org/display/Brightness';
const INTERFACE = 'org.display.Brightness';

// Axis bounds for the lux range the editor shows. The low end has to stay above
// a handful of lux or the curve cannot describe a lit room at all.
const AXIS_MIN = 50;
const AXIS_MAX = 100000;

// A press that moves less than this is a click (add a point), not a drag.
const CLICK_SLOP = 4;
// Grab radius around a control point, in pixels.
const HIT_RADIUS = 10;

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
        this._maxLux = 1000;
        this._dragIndex = -1;
        this._dragOrigin = [0, 0];

        // Called when an edit is finished and should reach the daemon.
        this.onCommit = null;

        this.set_draw_func((_area, cr, width, height) => this._draw(cr, width, height));

        // One gesture owns both moving and adding points. Running a separate
        // GestureClick alongside this made every press that began a drag also
        // insert a point, since both gestures saw the same press.
        const drag = new Gtk.GestureDrag();
        drag.connect('drag-begin', (_g, x, y) => {
            this._dragIndex = this._hitPoint(x, y);
            this._dragOrigin = [x, y];
        });
        drag.connect('drag-update', (_g, dx, dy) => this._onDragUpdate(dx, dy));
        drag.connect('drag-end', (_g, dx, dy) => this._onDragEnd(dx, dy));
        this.add_controller(drag);

        const right = new Gtk.GestureClick({button: 3});
        right.connect('pressed', (_g, _n, x, y) => {
            const i = this._hitPoint(x, y);
            if (i > 0 && i < this._points.length - 1) {
                this._points.splice(i, 1);
                this._commit();
            }
        });
        this.add_controller(right);
    }

    setPoints(points) {
        this._points = points.map(p => [p[0], p[1]]);
        this._sanitize();
        this.queue_draw();
    }

    getPoints() {
        this._sanitize();
        return this._points.map(([lux, bri]) => [lux, bri]);
    }

    getMaxLux() {
        return this._maxLux;
    }

    // suggestMaxLux picks an axis range that leaves the top control point
    // visible without welding it to the right edge.
    suggestMaxLux() {
        const top = this._points.length ? this._points[this._points.length - 1][0] : 0;
        return Math.max(AXIS_MIN, Math.min(AXIS_MAX, Math.round(top)));
    }

    setMaxLux(maxLux) {
        const next = Math.max(AXIS_MIN, Math.min(AXIS_MAX, Math.round(maxLux)));
        if (next === this._maxLux)
            return;
        this._maxLux = next;
        this._sanitize();
        this.queue_draw();
    }

    setLux(lux) {
        this._lux = lux;
        this.queue_draw();
    }

    // _sanitize forces the invariants the daemon validates: integer lux in
    // range, brightness 0-100, strictly increasing lux, an anchor at 0 lux and
    // at least two points. Rounding a pixel position used to be able to produce
    // two points at the same lux, which the daemon rejected outright.
    _sanitize() {
        const pts = this._points
            .map(([l, b]) => [
                Math.max(0, Math.min(this._maxLux, Math.round(l))),
                Math.max(0, Math.min(100, Math.round(b))),
            ])
            .sort((a, b) => a[0] - b[0]);

        const out = [];
        for (const p of pts) {
            if (out.length && p[0] <= out[out.length - 1][0]) {
                // Collided after clamping; the later point wins the brightness.
                out[out.length - 1][1] = p[1];
                continue;
            }
            out.push(p);
        }
        if (!out.length)
            out.push([0, 10]);
        if (out[0][0] !== 0)
            out.unshift([0, out[0][1]]);
        if (out.length < 2)
            out.push([this._maxLux, 100]);
        this._points = out;
    }

    _commit() {
        this._sanitize();
        this.queue_draw();
        if (this.onCommit)
            this.onCommit();
    }

    _pad() {
        return {l: 48, r: 16, t: 16, b: 36};
    }

    // Lux is mapped logarithmically: perceived brightness is roughly
    // logarithmic, and on a linear 0-1000 axis everything below 50 lux — where
    // an indoor room actually sits — was squeezed into a few pixels.
    _luxToNorm(lux) {
        return Math.log10(1 + Math.max(0, lux)) / Math.log10(1 + this._maxLux);
    }

    _normToLux(t) {
        return Math.pow(1 + this._maxLux, Math.max(0, Math.min(1, t))) - 1;
    }

    _toScreen(lux, bri, width, height) {
        const p = this._pad();
        const x = p.l + this._luxToNorm(lux) * (width - p.l - p.r);
        const y = p.t + (1 - bri / 100) * (height - p.t - p.b);
        return [x, y];
    }

    _fromScreen(x, y, width, height) {
        const p = this._pad();
        const plotW = Math.max(1, width - p.l - p.r);
        const plotH = Math.max(1, height - p.t - p.b);
        const lux = this._normToLux((x - p.l) / plotW);
        const bri = (1 - (y - p.t) / plotH) * 100;
        return [
            Math.round(Math.max(0, Math.min(this._maxLux, lux))),
            Math.round(Math.max(0, Math.min(100, bri))),
        ];
    }

    // _hitPoint returns the nearest control point within the grab radius, so
    // overlapping points at the low end do not always resolve to the first one.
    _hitPoint(x, y) {
        const w = this.get_width();
        const h = this.get_height();
        let best = -1;
        let bestDist = HIT_RADIUS * HIT_RADIUS;
        for (let i = 0; i < this._points.length; i++) {
            const [sx, sy] = this._toScreen(this._points[i][0], this._points[i][1], w, h);
            const dist = (sx - x) ** 2 + (sy - y) ** 2;
            if (dist <= bestDist) {
                best = i;
                bestDist = dist;
            }
        }
        return best;
    }

    _onDragUpdate(dx, dy) {
        if (this._dragIndex < 0)
            return;
        const w = this.get_width();
        const h = this.get_height();
        const [x0, y0] = this._dragOrigin;
        let [lux, bri] = this._fromScreen(x0 + dx, y0 + dy, w, h);
        const i = this._dragIndex;

        if (i === 0) {
            // The first point defines brightness in darkness; keep it at 0 lux.
            lux = 0;
        } else {
            const minLux = this._points[i - 1][0] + 1;
            // The top point moves freely up to the axis maximum. Pinning it to
            // the axis maximum — which was itself derived from that point — made
            // the lux at which full brightness is reached impossible to change.
            const maxLux = i === this._points.length - 1
                ? this._maxLux
                : this._points[i + 1][0] - 1;
            lux = minLux > maxLux
                ? this._points[i][0]
                : Math.max(minLux, Math.min(maxLux, lux));
        }
        this._points[i] = [lux, bri];
        this.queue_draw();
    }

    _onDragEnd(dx, dy) {
        const wasDrag = this._dragIndex >= 0;
        this._dragIndex = -1;

        if (wasDrag) {
            this._commit();
            return;
        }
        // Nothing grabbed: a press that barely moved is a click on empty space.
        if (Math.abs(dx) <= CLICK_SLOP && Math.abs(dy) <= CLICK_SLOP) {
            const [x, y] = this._dragOrigin;
            const [lux, bri] = this._fromScreen(x, y, this.get_width(), this.get_height());
            if (lux > 0 && lux <= this._maxLux) {
                this._points.push([lux, bri]);
                this._commit();
            }
        }
    }

    _xTicks() {
        const ticks = [];
        for (const lux of [0, 1, 10, 100, 1000, 10000, 100000]) {
            if (lux <= this._maxLux)
                ticks.push(lux);
        }
        if (ticks[ticks.length - 1] !== this._maxLux)
            ticks.push(this._maxLux);
        return ticks;
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
        for (const lux of this._xTicks()) {
            const [x] = this._toScreen(lux, 0, width, height);
            cr.moveTo(x, p.t);
            cr.lineTo(x, p.t + plotH);
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

        if (this._lux !== null) {
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
        for (const lux of this._xTicks()) {
            const [x] = this._toScreen(lux, 0, width, height);
            const label = `${lux}`;
            // Nudge the end labels inside the plot instead of measuring text.
            const offset = Math.min(Math.max(x - label.length * 3, p.l), width - p.r - label.length * 6);
            cr.moveTo(offset, height - 12);
            cr.showText(label);
        }
    }
});

export default class DisplayBrightnessPrefs extends ExtensionPreferences {
    fillPreferencesWindow(window) {
        window.set_default_size(560, 560);
        window.title = 'Lighting curve';

        const page = new Adw.PreferencesPage();
        const group = new Adw.PreferencesGroup({
            title: 'Lux → brightness',
            description: 'Drag points to reshape the curve. Click empty space to add a point; ' +
                'right-click a middle point to remove. Edits apply immediately.',
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
            wrap: true,
        });

        // Set while loading or clamping, so programmatic updates to the axis
        // control do not look like user edits and re-send the curve.
        let syncing = false;

        const setStatus = (text, isError) => {
            status.label = text;
            if (isError)
                status.add_css_class('error');
            else
                status.remove_css_class('error');
        };

        const applyCurve = async () => {
            const pts = curve.getPoints();
            const variant = new GLib.Variant('(a(uu))', [pts.map(([l, b]) => [l, b])]);
            try {
                await call('SetCurve', variant, null);
                setStatus(`Applied ${pts.length} points.`, false);
            } catch (e) {
                setStatus(`Could not apply: ${e.message}`, true);
            }
        };

        curve.onCommit = () => {
            if (!syncing)
                applyCurve();
        };

        const axisSpin = Gtk.SpinButton.new_with_range(AXIS_MIN, AXIS_MAX, 50);
        axisSpin.set_value(curve.getMaxLux());
        axisSpin.valign = Gtk.Align.CENTER;
        const axisRow = new Adw.ActionRow({
            title: 'Axis maximum (lux)',
            subtitle: 'Bright end of the graph. Raise it to place points at higher lux.',
        });
        axisRow.add_suffix(axisSpin);
        axisRow.activatable_widget = axisSpin;
        axisSpin.connect('value-changed', () => {
            if (syncing)
                return;
            const before = JSON.stringify(curve.getPoints());
            curve.setMaxLux(axisSpin.get_value());
            // Lowering the axis can clamp points, which is a real edit.
            if (JSON.stringify(curve.getPoints()) !== before)
                applyCurve();
        });

        const saveBtn = new Gtk.Button({
            label: 'Re-apply curve',
            css_classes: ['suggested-action'],
            halign: Gtk.Align.START,
            margin_top: 12,
        });
        saveBtn.connect('clicked', () => applyCurve());

        const reloadBtn = new Gtk.Button({
            label: 'Reload from daemon',
            halign: Gtk.Align.START,
            margin_top: 12,
        });

        const buttons = new Gtk.Box({
            orientation: Gtk.Orientation.HORIZONTAL,
            spacing: 8,
        });
        buttons.append(saveBtn);
        buttons.append(reloadBtn);

        const box = new Gtk.Box({
            orientation: Gtk.Orientation.VERTICAL,
            spacing: 4,
            margin_top: 8,
            margin_bottom: 8,
        });
        box.append(curve);
        box.append(luxLabel);
        box.append(buttons);
        box.append(status);
        group.add(box);
        page.add(group);

        const axisGroup = new Adw.PreferencesGroup();
        axisGroup.add(axisRow);
        page.add(axisGroup);

        window.add(page);

        const load = async () => {
            try {
                const [points] = await call('GetCurve', null, '(a(uu))');
                syncing = true;
                curve.setPoints(points);
                curve.setMaxLux(curve.suggestMaxLux());
                axisSpin.set_value(curve.getMaxLux());
                syncing = false;

                const [lux] = await call('GetLux', null, '(u)');
                curve.setLux(lux);
                luxLabel.label = `Lux: ${lux}`;
                setStatus('', false);
            } catch (e) {
                syncing = false;
                setStatus(`Could not load from daemon: ${e.message}`, true);
            }
        };
        load();
        reloadBtn.connect('clicked', () => load());

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
