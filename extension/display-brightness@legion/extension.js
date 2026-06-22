import GObject from 'gi://GObject';
import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as QuickSettings from 'resource:///org/gnome/shell/ui/quickSettings.js';

const BUS_NAME = 'org.display.Brightness';
const OBJECT_PATH = '/org/display/Brightness';
const INTERFACE = 'org.display.Brightness';

const DisplayBrightnessIndicator = GObject.registerClass(
class DisplayBrightnessIndicator extends QuickSettings.SystemIndicator {
    _init() {
        super._init();

        this._updating = false;
        this._serviceAvailable = false;

        this._slider = new QuickSettings.QuickSlider();
        this._slider.iconName = 'display-brightness-symbolic';

        this._slider.slider.connect('notify::value', () => {
            if (this._updating || !this._serviceAvailable)
                return;
            const percent = Math.round(this._slider.slider.value * 100);
            this._callMethod('SetBrightness', new GLib.Variant('(y)', [percent]));
        });

        this.quickSettingsItems.push(this._slider);

        this._watchName();
        this._subscribeSignal();
        this._syncBrightness();
    }

    _getProxyFlags() {
        return Gio.DBusProxyFlags.NONE;
    }

    _callMethod(method, params) {
        Gio.DBus.session.call(
            BUS_NAME,
            OBJECT_PATH,
            INTERFACE,
            method,
            params,
            null,
            Gio.DBusCallFlags.NONE,
            -1,
            null,
            (conn, result) => {
                try {
                    conn.call_finish(result);
                } catch (e) {
                    log(`display-brightness: ${method} failed: ${e.message}`);
                    this._setAvailable(false);
                }
            }
        );
    }

    _syncBrightness() {
        Gio.DBus.session.call(
            BUS_NAME,
            OBJECT_PATH,
            INTERFACE,
            'GetBrightness',
            null,
            new GLib.VariantType('(y)'),
            Gio.DBusCallFlags.NONE,
            -1,
            null,
            (conn, result) => {
                try {
                    const [, value] = conn.call_finish(result).deepUnpack();
                    this._setSliderValue(value);
                    this._setAvailable(true);
                } catch (e) {
                    log(`display-brightness: GetBrightness failed: ${e.message}`);
                    this._setAvailable(false);
                }
            }
        );
    }

    _setSliderValue(percent) {
        this._updating = true;
        this._slider.slider.value = percent / 100;
        this._updating = false;
    }

    _setAvailable(available) {
        this._serviceAvailable = available;
        this._slider.slider.reactive = available;
        this._slider.visible = true;
    }

    _watchName() {
        this._nameWatcher = Gio.DBus.session.watch_name(
            BUS_NAME,
            Gio.BusNameWatcherFlags.NONE,
            () => {
                this._setAvailable(true);
                this._refreshDisplays();
            },
            () => {
                this._setAvailable(false);
            }
        );
    }

    _refreshDisplays() {
        Gio.DBus.session.call(
            BUS_NAME,
            OBJECT_PATH,
            INTERFACE,
            'RefreshDisplays',
            null,
            new GLib.VariantType('(as)'),
            Gio.DBusCallFlags.NONE,
            -1,
            null,
            (conn, result) => {
                try {
                    conn.call_finish(result);
                    this._syncBrightness();
                } catch (e) {
                    log(`display-brightness: RefreshDisplays failed: ${e.message}`);
                    this._setAvailable(false);
                }
            }
        );
    }

    _subscribeSignal() {
        this._signalSub = Gio.DBus.session.signal_subscribe(
            BUS_NAME,
            INTERFACE,
            'BrightnessChanged',
            OBJECT_PATH,
            null,
            Gio.DBusSignalFlags.NONE,
            (_conn, _sender, _path, _iface, _signal, params) => {
                const [value] = params.deepUnpack();
                this._setSliderValue(value);
            }
        );
    }

    destroy() {
        if (this._signalSub)
            Gio.DBus.session.signal_unsubscribe(this._signalSub);
        if (this._nameWatcher)
            this._nameWatcher.cancel();
        super.destroy();
    }
});

export default class DisplayBrightnessExtension {
    enable() {
        this._indicator = new DisplayBrightnessIndicator();
        Main.panel.statusArea.quickSettings.addExternalIndicator(this._indicator);
    }

    disable() {
        if (this._indicator) {
            this._indicator.destroy();
            this._indicator = null;
        }
    }
}
