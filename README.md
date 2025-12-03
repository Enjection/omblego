# omblego

Cross-platform CLI tool to read blood pressure records from Omron Bluetooth LE devices.

Go rewrite of [omblepy](https://github.com/userx14/omblepy) with macOS support.

## Platform Support

| OS | Status | Notes |
|----|--------|-------|
| macOS | Supported | CoreBluetooth (requires Xcode) |
| Linux | Supported | BlueZ |
| Windows | Experimental | WinRT |

## Supported Devices

- HEM-7322T (M700 Intelli IT)
- HEM-7600T (Omron Evolv)
- HEM-6232T (wrist monitor)
- HEM-7530T (Omron Complete)
- HEM-7155T (M400/M4/X4 smart)
- HEM-7150T (BP7250)
- HEM-7342T (BP7450)
- HEM-7361T (M500/M7 Intelli IT)

## Installation

### From Source

```bash
# Requires Go 1.21+
git clone <repo-url>
cd omblego
go build -o omblego ./cmd/omblego
```

### macOS Requirements

- Xcode (for CoreBluetooth CGo bindings)
- Bluetooth permission in System Preferences

## Usage

### First-Time Pairing

Put your Omron device in pairing mode (hold Bluetooth button until `-P-` displays):

```bash
omblego -d HEM-7322T -p
```

Accept any OS Bluetooth pairing dialogs when prompted.

### Reading Records

After pairing, read records without the `-p` flag:

```bash
omblego -d HEM-7322T
```

### Options

```
-d, --device string   Device model name (required)
-m, --mac string      Bluetooth MAC address (skip device scan)
-p, --pair            Enable pairing mode (first-time setup)
-n, --newRecOnly      Only read new/unread records
-t, --timeSync        Synchronize device time with system
-o, --output string   Output format: csv, json, or both (default: csv)
    --debug           Enable debug logging
```

### Examples

```bash
# Pair a new device
omblego -d HEM-7322T -p

# Read all records to CSV
omblego -d HEM-7322T

# Read only new records with time sync
omblego -d HEM-7322T -n -t

# Output both CSV and JSON
omblego -d HEM-7322T -o both

# Skip device scan with known MAC address
omblego -d HEM-7322T -m "AA:BB:CC:DD:EE:FF"

# List supported devices
omblego list-devices
```

## Output Formats

### CSV (user1.csv, user2.csv)

```csv
datetime,dia,sys,bpm,mov,ihb
2024-01-15 08:30:00,80,120,72,0,0
2024-01-16 07:45:30,82,125,68,1,0
```

### JSON (ubpm.json) - UBPM Compatible

```json
{
    "UBPM": {
        "U1": [
            {
                "date": "15.01.2024",
                "time": "08:30:00",
                "sys": 120,
                "dia": 80,
                "bpm": 72,
                "ihb": 0,
                "mov": 0
            }
        ]
    }
}
```

## Troubleshooting

### macOS

- Ensure Bluetooth is enabled in System Preferences
- Grant Bluetooth permission to Terminal/iTerm
- If pairing fails, remove the device from System Preferences > Bluetooth first

### Linux

- Ensure BlueZ is installed: `apt install bluez`
- May need to run with sudo for Bluetooth access
- For multi-adapter systems, see [omblepy documentation](https://github.com/userx14/omblepy)

### Windows

- Requires Windows 10/11 with Bluetooth 4.2+
- WinRT support is experimental

## Credits

- Original Python implementation: [omblepy](https://github.com/userx14/omblepy) by userx14
- UBPM protocol research: [LazyT/ubpm](https://codeberg.org/LazyT/ubpm)
- BLE library: [tinygo-org/bluetooth](https://github.com/tinygo-org/bluetooth)

## License

Same as omblepy (check original repository for license terms).
