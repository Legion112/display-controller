package dbus

import (
	"context"
	"fmt"
	"strconv"

	"github.com/godbus/dbus/v5"
	"github.com/legion/display/internal/brightness"
)

const (
	BusName    = "org.display.Brightness"
	ObjectPath = "/org/display/Brightness"
	Interface  = "org.display.Brightness"
)

// Service implements org.display.Brightness on the session bus.
type Service struct {
	conn       *dbus.Conn
	controller *brightness.Controller
}

// NewService wires the D-Bus service to a brightness controller.
func NewService(conn *dbus.Conn, controller *brightness.Controller) *Service {
	s := &Service{
		conn:       conn,
		controller: controller,
	}
	controller.SetChangeHandler(func(percent int) {
		s.emitBrightnessChanged(byte(percent))
	})
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
	_ = s.conn.Emit(dbus.ObjectPath(ObjectPath), Interface, "BrightnessChanged", value)
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
		return fmt.Errorf("bus name %s already taken", BusName)
	}
	return nil
}
