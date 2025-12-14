# omblego

CLI tool to read blood pressure records from Omron Bluetooth LE devices.

Go rewrite of [omblepy](https://github.com/userx14/omblepy) with macOS support.

## Supported Devices

- BP7000, BP5250, EVOLV
- HEM-7322T (M700 Intelli IT)
- HEM-7600T (Omron Evolv)
- HEM-6232T (wrist monitor)
- HEM-7530T (Omron Complete)
- HEM-7155T, HEM-7150T, HEM-7342T, HEM-7361T

## Installation

```bash
git clone <repo-url>
cd omblego
go build -o omblego ./cmd/omblego
```

Requirements:
- Go 1.21+
- macOS: Xcode (for CoreBluetooth)
- Linux: BlueZ (`apt install bluez`)

## Quick Start

### 1. Pair your device

Put device in pairing mode (hold Bluetooth button until -P- displays):

```bash
omblego pair -d BP7000 --interactive
```

### 2. Sync records

```bash
omblego sync -d BP7000 --auto
```

## Commands

```
omblego pair          Pair with a new device
omblego sync          Sync blood pressure records
omblego daemon        Run as daemon (periodic sync)
omblego scan          Scan for nearby devices
omblego list-devices  List supported devices
omblego version       Print version
```

### Pair

```bash
omblego pair -d BP7000 --interactive     # Scan and select device
omblego pair -d BP7000 --mac AA:BB:CC:DD:EE:FF
```

### Sync

```bash
omblego sync -d BP7000 --auto            # Wait for device (macOS)
omblego sync -d BP7000 --auto --time-sync
omblego sync -d BP7000 --auto --new-only
omblego sync -d BP7000 --uuid <device-uuid>
omblego sync -d BP7000 -o json           # Output format: csv, json, both
```

### Daemon

```bash
omblego daemon -d BP7000 --interval 3h
omblego daemon -d BP7000 --interval 3h --influx-url http://localhost:8086 \
  --influx-token TOKEN --influx-org myorg --influx-bucket health
```

Signals:
- SIGHUP: Force immediate sync
- SIGTERM/SIGINT: Graceful shutdown

## Configuration

Config file search order:
1. Explicit path: `omblego --config /path/to/config.yaml`
2. User config: `~/.config/omblego/config.yaml`
3. System config: `/etc/omblego/config.yaml`

Example `~/.config/omblego/config.yaml`:

```yaml
device:
  model: BP7000
  uuid: auto

daemon:
  interval: 3h
  time_sync: true

logging:
  level: info
  format: text
  output: stderr

outputs:
  - type: csv
    enabled: true
    settings:
      directory: "."
      file_pattern: "user{user}.csv"

  - type: influxdb
    enabled: false
    settings:
      url: http://localhost:8086
      token: ${INFLUXDB_TOKEN}
      org: personal
      bucket: health
```

## Output Formats

### CSV (user1.csv, user2.csv)

```csv
datetime,dia,sys,bpm,mov,ihb
2024-01-15 08:30:00,80,120,72,0,0
```

### JSON (ubpm.json)

```json
{
    "UBPM": {
        "U1": [{"date": "15.01.2024", "time": "08:30:00", "sys": 120, "dia": 80, "bpm": 72}]
    }
}
```

## Troubleshooting

**macOS:**
- Grant Bluetooth permission to Terminal
- If pairing fails, remove device from System Preferences > Bluetooth first

**Linux:**
- May need sudo for Bluetooth access
- Ensure BlueZ is installed

## Adding New Devices

1. Add driver in `internal/device/devices/devices.go`:

```go
type MyDevice struct {
    config device.Config
}

func NewMyDevice() device.Driver {
    return &MyDevice{
        config: device.Config{
            Name:                  "MY-DEVICE",
            Protocol:              device.ProtocolOmronCustom, // or ProtocolStandardGATT
            BLENamePrefix:         "BLEsmart_00000XXX",
            Endianness:            device.BigEndian,
            UserStartAddresses:    []uint16{0x02ac},
            RecordsPerUser:        []int{100},
            RecordByteSize:        0x0e,
            TransmissionBlockSize: 0x38,
            // ... other config
        },
    }
}

func (d *MyDevice) GetConfig() device.Config { return d.config }
func (d *MyDevice) ParseRecord(data []byte) (*device.BloodPressureRecord, error) {
    // Parse device-specific record format
}
func (d *MyDevice) BuildTimeSyncBytes(settings []byte, t time.Time) ([]byte, error) {
    // Build time sync payload
}
```

2. Register in `init()`:

```go
func init() {
    device.Register("my-device", NewMyDevice)
}
```

Protocol types:
- `ProtocolOmronCustom`: EEPROM-based (HEM-* devices)
- `ProtocolStandardGATT`: Standard BLE Blood Pressure Service (BP5250)

## Adding New Output Sinks

1. Create backend in `internal/output/mybackend.go`:

```go
type MyBackend struct {
    name string
    // your config fields
}

func NewMyBackend(name string, settings map[string]interface{}) (*MyBackend, error) {
    // parse settings, validate config
    return &MyBackend{name: name}, nil
}

func (b *MyBackend) Name() string { return b.name }
func (b *MyBackend) Type() string { return "mybackend" }

func (b *MyBackend) Write(ctx context.Context, records [][]Record, meta Metadata) error {
    for userIdx, userRecords := range records {
        for _, rec := range userRecords {
            // write rec.Systolic, rec.Diastolic, rec.Pulse, rec.Timestamp, etc.
        }
    }
    return nil
}

func (b *MyBackend) Health(ctx context.Context) error { return nil }
func (b *MyBackend) Close() error { return nil }
```

2. Register in `internal/output/factory.go`:

```go
func init() {
    Register("mybackend", func(name string, settings map[string]interface{}) (Backend, error) {
        return NewMyBackend(name, settings)
    })
}
```

3. Use in config:

```yaml
outputs:
  - type: mybackend
    enabled: true
    settings:
      my_option: value
```

## Credits

- Original: [omblepy](https://github.com/userx14/omblepy) by userx14
- Protocol: [LazyT/ubpm](https://codeberg.org/LazyT/ubpm)
- Protocol reference: [StanfordSpezi/SpeziDevices](https://github.com/StanfordSpezi/SpeziDevices)
- BLE: [tinygo-org/bluetooth](https://github.com/tinygo-org/bluetooth)
