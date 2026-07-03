//! VEML7700 ambient light sensor on Arduino Nano.
//!
//! Wiring:
//!   VEML7700 VCC -> 5V, GND -> GND, SDA -> A4, SCL -> A5
//!
//! Serial protocol (57600 baud):
//!   Host sends `R` or newline -> device replies `LUX <integer>\r\n` or `ERR sensor\r\n`

#![no_std]
#![no_main]

use arduino_hal::prelude::*;
use panic_halt as _;
use unwrap_infallible::UnwrapInfallible;
use veml7700::{Gain, IntegrationTime, Veml7700};

#[arduino_hal::entry]
fn main() -> ! {
    let dp = arduino_hal::Peripherals::take().unwrap();
    let pins = arduino_hal::pins!(dp);
    let mut serial = arduino_hal::default_serial!(dp, pins, 57600);

    let i2c = arduino_hal::I2c::new(
        dp.TWI,
        pins.a4.into_pull_up_input(),
        pins.a5.into_pull_up_input(),
        400_000,
    );

    let mut sensor = Veml7700::new(i2c);
    if sensor.enable().is_err()
        || sensor.set_integration_time(IntegrationTime::_100ms).is_err()
        || sensor.set_gain(Gain::One).is_err()
    {
        ufmt::uwriteln!(&mut serial, "ERR sensor\r").unwrap_infallible();
        loop {
            arduino_hal::delay_ms(1000);
        }
    }

    // Datasheet: wait 4 ms after enable before first measurement.
    arduino_hal::delay_ms(4);

    ufmt::uwriteln!(&mut serial, "READY\r").unwrap_infallible();

    loop {
        match serial.read() {
            Ok(b) if b == b'R' || b == b'\n' || b == b'\r' => respond_lux(&mut serial, &mut sensor),
            Ok(_) => {}
            Err(nb::Error::WouldBlock) => {}
        }
    }
}

fn respond_lux<I2C>(serial: &mut impl ufmt::uWrite, sensor: &mut Veml7700<I2C>)
where
    I2C: embedded_hal::i2c::I2c,
{
    match sensor.read_lux() {
        Ok(lux) => {
            let lux_int = lux.max(0.0) as u32;
            let _ = ufmt::uwriteln!(serial, "LUX {}\r", lux_int);
        }
        Err(_) => {
            let _ = ufmt::uwriteln!(serial, "ERR sensor\r");
        }
    }
}
