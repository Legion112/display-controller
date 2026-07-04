//! VEML7700 ambient light sensor on Arduino Nano.
//!
//! Wiring:
//!   VEML7700 VIN -> 5V, GND -> GND, SDA -> A4, SCL -> A5
//!   4.7 kΩ pull-ups on SDA/SCL to 5V (or 3VO) recommended at 400 kHz.
//!
//! Serial (57600 baud):
//!   Boot: I2C scan -> Found 0x.. lines -> READY or ERR init N
//!   R/newline: LUX n | ERR read | ERR no sensor
//!   S: re-run I2C scan

#![no_std]
#![no_main]

use arduino_hal::prelude::*;
use arduino_hal::I2c;
use embedded_hal::i2c::I2c as I2cBus;
use panic_halt as _;
use unwrap_infallible::UnwrapInfallible;
use veml7700::{Gain, IntegrationTime, Veml7700};

const I2C_HZ: u32 = 400_000;
const VEML7700_ADDR: u8 = 0x10;

#[arduino_hal::entry]
fn main() -> ! {
    let dp = arduino_hal::Peripherals::take().unwrap();
    let pins = arduino_hal::pins!(dp);
    let mut serial = arduino_hal::default_serial!(dp, pins, 57600);

    let mut i2c = I2c::new(
        dp.TWI,
        pins.a4.into_pull_up_input(),
        pins.a5.into_pull_up_input(),
        I2C_HZ,
    );

    scan_i2c(&mut i2c, &mut serial);

    let mut sensor = Veml7700::new(i2c);
    let mut ready = match init_sensor(&mut sensor) {
        Ok(()) => {
            ufmt::uwriteln!(&mut serial, "READY\r").unwrap_infallible();
            true
        }
        Err(step) => {
            ufmt::uwriteln!(&mut serial, "ERR init {}\r", step as char).unwrap_infallible();
            false
        }
    };

    loop {
        match serial.read() {
            Ok(b'S') => {
                i2c = sensor.destroy();
                scan_i2c(&mut i2c, &mut serial);
                sensor = Veml7700::new(i2c);
            }
            Ok(b) if b == b'R' || b == b'\n' || b == b'\r' => {
                if !ready {
                    match init_sensor(&mut sensor) {
                        Ok(()) => {
                            ready = true;
                            ufmt::uwriteln!(&mut serial, "READY\r").unwrap_infallible();
                        }
                        Err(_) => {
                            ufmt::uwriteln!(&mut serial, "ERR no sensor\r").unwrap_infallible();
                            continue;
                        }
                    }
                }
                respond_lux(&mut serial, &mut sensor);
            }
            Ok(_) => {}
            Err(nb::Error::WouldBlock) => {}
        }
    }
}

fn scan_i2c<W: ufmt::uWrite>(i2c: &mut I2c, serial: &mut W) {
    let _ = ufmt::uwriteln!(serial, "I2C scan\r");
    let mut count = 0u8;
    for addr in 1u8..127u8 {
        if I2cBus::write(i2c, addr, &[]).is_ok() {
            let _ = ufmt::uwriteln!(serial, "Found 0x{:x}\r", addr);
            count += 1;
        }
    }
    if count == 0 {
        let _ = ufmt::uwriteln!(serial, "Found none\r");
    }
    if I2cBus::write(i2c, VEML7700_ADDR, &[]).is_ok() {
        let _ = ufmt::uwriteln!(serial, "VEML7700 0x10 ok\r");
    } else {
        let _ = ufmt::uwriteln!(serial, "VEML7700 0x10 missing\r");
    }
    let _ = ufmt::uwriteln!(serial, "scan done\r");
}

fn init_sensor<I2C>(sensor: &mut Veml7700<I2C>) -> Result<(), u8>
where
    I2C: I2cBus,
{
    if sensor.enable().is_err() {
        return Err(b'1');
    }
    if sensor.set_integration_time(IntegrationTime::_100ms).is_err() {
        return Err(b'2');
    }
    if sensor.set_gain(Gain::One).is_err() {
        return Err(b'3');
    }
    arduino_hal::delay_ms(104);
    Ok(())
}

fn respond_lux<I2C>(serial: &mut impl ufmt::uWrite, sensor: &mut Veml7700<I2C>)
where
    I2C: I2cBus,
{
    match sensor.read_lux() {
        Ok(lux) => {
            let lux_int = lux.max(0.0) as u32;
            let _ = ufmt::uwriteln!(serial, "LUX {}\r", lux_int);
        }
        Err(_) => {
            let _ = ufmt::uwriteln!(serial, "ERR read\r");
        }
    }
}
