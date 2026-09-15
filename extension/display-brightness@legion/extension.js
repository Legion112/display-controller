import GObject from 'gi://GObject';
import GLib from 'gi://GLib';
import Gio from 'gi://Gio';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as QuickSettings from 'resource:///org/gnome/shell/ui/quickSettings.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const BUS_NAME = 'org.display.Brightness';
const OBJECT_PATH = '/org/display/Brightness';
const INTERFACE = 'org.display.Brightness';

const SYNC_RETRY_INTERVAL_SEC = 3;
const SYNC_RETRY_MAX = 10;

const DisplayBrightnessIndicator = GObject.registerClass(
class DisplayBrightnessIndicator extends QuickSettings.SystemIndicator {
    _init(extension) {
        super._init();

        this._extension = extension;
        this._updating = false;
        this._dragging = false;
        this._serviceAvailable = false;
        this._autoEnabled = false;
        this._syncingAuto = false;
        this._syncRetries = 0;
        this._syncRetrySource = null;
        this._signalSubs = [];

        this._slider = new QuickSettings.QuickSlider();
        this._slider.iconName = 'display-brightness-symbolic';

        this._slider.slider.connect('notify::value', () => {
            if (this._updating || !this._serviceAvailable || this._autoEnabled)
                return;
            const percent = Math.round(this._slider.slider.value * 100);
            this._callMethod('SetBrightness', new GLib.Variant('(y)', [percent]));
        });

        this._slider.slider.connect('drag-begin', () => {
            this._dragging = true;
        });
        this._slider.slider.connect('drag-end', () => {
            this._dragging = false;
        });

        this._autoToggle = new QuickSettings.QuickMenuToggle({
            title: 'Auto',
            subtitle: 'Brightness',
            iconName: 'weather-clear-symbolic',
            toggleMode: true,
        });
        this._autoToggle.menu.setHeader('weather-clear-symbolic', 'Auto brightness');
        this._autoToggle.menu.addAction('Edit lighting curve…', () => {
            this._extension.openPreferences();
        });
        this._autoToggle.connect('notify::checked', () => {
            if (!this._serviceAvailable || this._syncingAuto)
                return;
            this._callMethod('SetAutoBrightness', new GLib.Variant('(b)', [this._autoToggle.checked]));
        });

        // Keep clickable even before the first name-owner callback.
        this._autoToggle.reactive = true;
        this.quickSettingsItems.push(this._slider, this._autoToggle);

        this._watchName();
        this._subscribeSignals();
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

    _clearSyncRetry() {
        if (this._syncRetrySource) {
            GLib.source_remove(this._syncRetrySource);
            this._syncRetrySource = null;
        }
        this._syncRetries = 0;
    }

    _scheduleSyncRetry() {
        if (this._syncRetrySource || this._syncRetries >= SYNC_RETRY_MAX)
            return;

        this._syncRetries++;
        this._syncRetrySource = GLib.timeout_add_seconds(
            GLib.PRIORITY_DEFAULT,
            SYNC_RETRY_INTERVAL_SEC,
            () => {
                this._syncRetrySource = null;
                this._refreshDisplays();
                return GLib.SOURCE_REMOVE;
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
                    const [value] = conn.call_finish(result).deepUnpack();
                    this._setSliderValue(value);
                    this._setAvailable(true);
                    this._clearSyncRetry();
                } catch (e) {
                    log(`display-brightness: GetBrightness failed: ${e.message}`);
                    this._setAvailable(false);
                    this._scheduleSyncRetry();
                }
            }
        );
    }

    _syncAuto() {
        Gio.DBus.session.call(
            BUS_NAME,
            OBJECT_PATH,
            INTERFACE,
            'GetAutoBrightness',
            null,
            new GLib.VariantType('(b)'),
            Gio.DBusCallFlags.NONE,
            -1,
            null,
            (conn, result) => {
                try {
                    const [enabled] = conn.call_finish(result).deepUnpack();
                    this._setAutoEnabled(enabled);
                } catch (e) {
                    log(`display-brightness: GetAutoBrightness failed: ${e.message}`);
                }
            }
        );
    }

    _setSliderValue(percent) {
        if (!Number.isFinite(percent))
            return;
        percent = Math.max(0, Math.min(100, percent));
        this._updating = true;
        this._slider.slider.value = percent / 100;
        this._updating = false;
    }

    _setAutoEnabled(enabled) {
        this._autoEnabled = !!enabled;
        this._syncingAuto = true;
        this._autoToggle.checked = this._autoEnabled;
        this._syncingAuto = false;
        this._slider.slider.reactive = this._serviceAvailable && !this._autoEnabled;
    }

    _setAvailable(available) {
        this._serviceAvailable = available;
        this._slider.slider.reactive = available && !this._autoEnabled;
        this._slider.visible = true;
        this._autoToggle.reactive = available;
    }

    _watchName() {
        this._nameWatcher = Gio.DBus.session.watch_name(
            BUS_NAME,
            Gio.BusNameWatcherFlags.NONE,
            () => {
                this._setAvailable(true);
                this._refreshDisplays();
                this._syncAuto();
            },
            () => {
                this._setAvailable(false);
                this._clearSyncRetry();
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
                    this._scheduleSyncRetry();
                }
            }
        );
    }

    _subscribeSignals() {
        const sub = (signal, cb) => {
            const id = Gio.DBus.session.signal_subscribe(
                BUS_NAME,
                INTERFACE,
                signal,
                OBJECT_PATH,
                null,
                Gio.DBusSignalFlags.NONE,
                (_conn, _sender, _path, _iface, _sig, params) => cb(params)
            );
            this._signalSubs.push(id);
        };

        sub('BrightnessChanged', params => {
            const [value] = params.deepUnpack();
            if (!this._dragging)
                this._setSliderValue(value);
            this._setAvailable(true);
            this._clearSyncRetry();
        });
        sub('AutoBrightnessChanged', params => {
            const [enabled] = params.deepUnpack();
            this._setAutoEnabled(enabled);
        });
    }

    destroy() {
        this._clearSyncRetry();
        for (const id of this._signalSubs)
            Gio.DBus.session.signal_unsubscribe(id);
        this._signalSubs = [];
        if (this._nameWatcher)
            this._nameWatcher.cancel();
        super.destroy();
    }
});

export default class DisplayBrightnessExtension extends Extension {
    enable() {
        this._indicator = new DisplayBrightnessIndicator(this);
        Main.panel.statusArea.quickSettings.addExternalIndicator(this._indicator);
    }

    disable() {
        if (this._indicator) {
            this._indicator.destroy();
            this._indicator = null;
        }
    }
}
