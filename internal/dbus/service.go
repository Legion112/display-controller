package dbus

import (
	"context"
	"fmt"
	"strconv"

	"github.com/godbus/dbus/v5"
	"github.com/legion/display/internal/autobrightness"
	"github.com/legion/display/internal/brightness"
	"github.com/legion/display/internal/config"
)

const (
	BusName    = "org.display.Brightness"
	ObjectPath = "/org/display/Brightness"
	Interface  = "org.display.Brightness"
)

// CurvePoint is the D-Bus struct for a(uu) curve points.
type CurvePoint struct {
	Lux        uint32
	Brightness uint32
}

// Service implements org.display.Brightness on the session bus.
type Service struct {
	conn       *dbus.Conn
	controller *brightness.Controller
	auto       *autobrightness.Manager
}

// NewService wires the D-Bus service to a brightness controller and auto manager.
func NewService(conn *dbus.Conn, controller *brightness.Controller, auto *autobrightness.Manager) *Service {
	s := &Service{
		conn:       conn,
		controller: controller,
		auto:       auto,
	}
	controller.SetChangeHandler(func(percent int) {
		s.emitBrightnessChanged(byte(percent))
	})
	if auto != nil {
		auto.SetAutoHandler(func(enabled bool) {
			s.emitAutoBrightnessChanged(enabled)
		})
		auto.SetLuxHandler(func(lux uint32) {
			s.emitLuxChanged(lux)
		})
	}
	return s
}

// Export registers the service on the connection.
func (s *Service) Export() error {
	return s.conn.Export(s, dbus.ObjectPath(ObjectPath), Interface)
}

func (s *Service) emitBrightnessChanged(value byte) {
	if s.conn == nil {
		return
	}
	// godbus Emit expects a single "interface.member" name.
	_ = s.conn.Emit(dbus.ObjectPath(ObjectPath), Interface+".BrightnessChanged", value)
}

func (s *Service) emitAutoBrightnessChanged(enabled bool) {
	if s.conn == nil {
		return
	}
	_ = s.conn.Emit(dbus.ObjectPath(ObjectPath), Interface+".AutoBrightnessChanged", enabled)
}

func (s *Service) emitLuxChanged(lux uint32) {
	if s.conn == nil {
		return
	}
	_ = s.conn.Emit(dbus.ObjectPath(ObjectPath), Interface+".LuxChanged", lux)
}

// EmitCurrentBrightness reads hardware brightness and notifies subscribers.
func (s *Service) EmitCurrentBrightness(ctx context.Context) {
	percent, err := s.controller.GetBrightness(ctx)
	if err != nil {
		return
	}
	s.emitBrightnessChanged(byte(percent))
}

// GetBrightness returns average brightness 0-100.
func (s *Service) GetBrightness() (byte, *dbus.Error) {
	percent, err := s.controller.GetBrightness(context.Background())
	if err != nil {
		return 0, dbus.MakeFailedError(err)
	}
	return byte(percent), nil
}

// SetBrightness sets all displays to the same brightness percent.
func (s *Service) SetBrightness(value byte) *dbus.Error {
	s.controller.SetBrightness(int(value))
	return nil
}

// RefreshDisplays re-detects DDC/CI displays.
func (s *Service) RefreshDisplays() ([]string, *dbus.Error) {
	displays, err := s.controller.RefreshDisplays(context.Background())
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	return intSliceToStringSlice(displays), nil
}

// GetDisplays returns cached display numbers.
func (s *Service) GetDisplays() ([]string, *dbus.Error) {
	return intSliceToStringSlice(s.controller.GetDisplays()), nil
}

// GetAutoBrightness returns whether auto mode is enabled.
func (s *Service) GetAutoBrightness() (bool, *dbus.Error) {
	if s.auto == nil {
		return false, nil
	}
	return s.auto.AutoEnabled(), nil
}

// SetAutoBrightness enables or disables auto mode (persisted).
func (s *Service) SetAutoBrightness(enabled bool) *dbus.Error {
	if s.auto == nil {
		return dbus.MakeFailedError(fmt.Errorf("auto-brightness unavailable"))
	}
	if err := s.auto.SetAuto(enabled); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// GetCurve returns lux→brightness control points.
func (s *Service) GetCurve() ([]CurvePoint, *dbus.Error) {
	if s.auto == nil {
		return nil, dbus.MakeFailedError(fmt.Errorf("auto-brightness unavailable"))
	}
	pts := s.auto.Curve()
	out := make([]CurvePoint, len(pts))
	for i, p := range pts {
		out[i] = CurvePoint{Lux: p.Lux, Brightness: p.Brightness}
	}
	return out, nil
}

// SetCurve replaces the lighting curve (persisted).
func (s *Service) SetCurve(points []CurvePoint) *dbus.Error {
	if s.auto == nil {
		return dbus.MakeFailedError(fmt.Errorf("auto-brightness unavailable"))
	}
	cfgPts := make([]config.Point, len(points))
	for i, p := range points {
		cfgPts[i] = config.Point{Lux: p.Lux, Brightness: p.Brightness}
	}
	if err := s.auto.SetCurve(cfgPts); err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// GetLux returns the last measured lux (0 if none yet).
func (s *Service) GetLux() (uint32, *dbus.Error) {
	if s.auto == nil {
		return 0, nil
	}
	return s.auto.LastLux(), nil
}

func intSliceToStringSlice(values []int) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strconv.Itoa(v)
	}
	return out
}

// AcquireName requests the well-known bus name.
func AcquireName(conn *dbus.Conn) error {
	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name %s already taken", BusName)
	}
	return nil
}
