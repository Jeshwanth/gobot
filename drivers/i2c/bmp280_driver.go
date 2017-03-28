package i2c

import (
	"bytes"
	"encoding/binary"

	"gobot.io/x/gobot"
)

const bmp280RegisterCalib00 = 0x88
const bme280RegisterPressureMSB = 0xf7

type bmp280CalibrationCoefficients struct {
	t1 uint16
	t2 int16
	t3 int16
	p1 uint16
	p2 int16
	p3 int16
	p4 int16
	p5 int16
	p6 int16
	p7 int16
	p8 int16
	p9 int16
}

// BMP280Driver is a driver for the BMP280 temperature/pressure sensor
type BMP280Driver struct {
	name       string
	connector  Connector
	connection Connection
	Config

	tpc *bmp280CalibrationCoefficients
}

// NewBMP280Driver creates a new driver with specified i2c interface.
// Params:
//		conn Connector - the Adaptor to use with this Driver
//
// Optional params:
//		i2c.WithBus(int):	bus to use with this driver
//		i2c.WithAddress(int):	address to use with this driver
//
func NewBMP280Driver(c Connector, options ...func(Config)) *BMP280Driver {
	b := &BMP280Driver{
		name:      gobot.DefaultName("BMP280"),
		connector: c,
		Config:    NewConfig(),
		tpc:       &bmp280CalibrationCoefficients{},
	}

	for _, option := range options {
		option(b)
	}

	// TODO: expose commands to API
	return b
}

// Name returns the name of the device.
func (d *BMP280Driver) Name() string {
	return d.name
}

// SetName sets the name of the device.
func (d *BMP280Driver) SetName(n string) {
	d.name = n
}

// Connection returns the connection of the device.
func (d *BMP280Driver) Connection() gobot.Connection {
	return d.connector.(gobot.Connection)
}

// Start initializes the BMP280 and loads the calibration coefficients.
func (d *BMP280Driver) Start() (err error) {
	bus := d.GetBusOrDefault(d.connector.GetDefaultBus())
	address := d.GetAddressOrDefault(bmp180Address)

	if d.connection, err = d.connector.GetConnection(address, bus); err != nil {
		return err
	}

	if err := d.initialization(); err != nil {
		return err
	}

	return nil
}

// Halt halts the device.
func (d *BMP280Driver) Halt() (err error) {
	return nil
}

// Temperature returns the current temperature, in celsius degrees.
func (d *BMP280Driver) Temperature() (temp float32, err error) {
	var rawT int32
	if rawT, _, err = d.rawTempPress(); err != nil {
		return 0.0, err
	}
	temp, _ = d.calculateTemp(rawT)
	return
}

// Pressure returns the current barometric pressure, in Pa
func (d *BMP280Driver) Pressure() (press float32, err error) {
	var rawT, rawP int32
	if rawT, rawP, err = d.rawTempPress(); err != nil {
		return 0.0, err
	}
	_, tFine := d.calculateTemp(rawT)
	return d.calculatePress(rawP, tFine), nil
}

// initialization reads the calibration coefficients.
func (d *BMP280Driver) initialization() (err error) {
	// TODO: set sleep mode here...

	var coefficients []byte
	if coefficients, err = d.read(bmp280RegisterCalib00, 26); err != nil {
		return err
	}
	buf := bytes.NewBuffer(coefficients)
	binary.Read(buf, binary.LittleEndian, &d.tpc.t1)
	binary.Read(buf, binary.LittleEndian, &d.tpc.t2)
	binary.Read(buf, binary.LittleEndian, &d.tpc.t3)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p1)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p2)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p3)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p4)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p5)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p6)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p7)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p8)
	binary.Read(buf, binary.LittleEndian, &d.tpc.p9)

	// TODO: set usage mode here...
	// TODO: set default sea level here

	return nil
}

func (d *BMP280Driver) rawTempPress() (temp int32, press int32, err error) {
	var data []byte
	var tp0, tp1, tp2, tp3, tp4, tp5 byte

	if data, err = d.read(bme280RegisterPressureMSB, 6); err != nil {
		return 0, 0, err
	}
	buf := bytes.NewBuffer(data)
	binary.Read(buf, binary.LittleEndian, &tp0)
	binary.Read(buf, binary.LittleEndian, &tp1)
	binary.Read(buf, binary.LittleEndian, &tp2)
	binary.Read(buf, binary.LittleEndian, &tp3)
	binary.Read(buf, binary.LittleEndian, &tp4)
	binary.Read(buf, binary.LittleEndian, &tp5)

	temp = ((int32(tp5) >> 4) | (int32(tp4) << 4) | (int32(tp3) << 12))
	press = ((int32(tp2) >> 4) | (int32(tp1) << 4) | (int32(tp0) << 12))

	return
}

func (d *BMP280Driver) calculateTemp(rawTemp int32) (float32, int32) {
	tcvar1 := ((float32(rawTemp) / 16384.0) - (float32(d.tpc.t1) / 1024.0)) * float32(d.tpc.t2)
	tcvar2 := (((float32(rawTemp) / 131072.0) - (float32(d.tpc.t1) / 8192.0)) * ((float32(rawTemp) / 131072.0) - float32(d.tpc.t1)/8192.0)) * float32(d.tpc.t3)
	temperatureComp := (tcvar1 + tcvar2) / 5120.0

	tFine := int32(tcvar1 + tcvar2)
	return temperatureComp, tFine
}

func (d *BMP280Driver) calculatePress(rawPress int32, tFine int32) float32 {
	pcvar1 := (float32(tFine) / 2.0) - 64000.0
	pcvar2 := pcvar1 * pcvar1 * (float32(d.tpc.p6)) / 32768.0
	pcvar2 = pcvar2 + pcvar1*(float32(d.tpc.p5))*2.0
	pcvar2 = (pcvar2 / 4.0) + (float32(d.tpc.p4) * 65536.0)
	pcvar1 = (float32(d.tpc.p3)*pcvar1*pcvar1/524288.0 + float32(d.tpc.p2)*pcvar1) / 524288.0
	pcvar1 = (1.0 + pcvar1/32768.0) * (float32(d.tpc.p1))
	if pcvar1 == 0.0 { // avoid divide by zero
		return 0.0
	}
	pressureComp := 1048576.0 - float32(rawPress)
	pressureComp = (pressureComp - (pcvar2 / 4096.0)) * (6250.0 / pcvar1)
	pcvar1 = float32(d.tpc.p9) * pressureComp * pressureComp / 2147483648.0
	pcvar2 = pressureComp * float32(d.tpc.p8) / 32768.0
	pressureComp = pressureComp + (pcvar1+pcvar2+float32(d.tpc.p7))/16.0

	return pressureComp
}

func (d *BMP280Driver) read(address byte, n int) ([]byte, error) {
	if _, err := d.connection.Write([]byte{address}); err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	bytesRead, err := d.connection.Read(buf)
	if bytesRead != n || err != nil {
		return nil, err
	}
	return buf, nil
}
