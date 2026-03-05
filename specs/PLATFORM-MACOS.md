# macOS Platform Implementation

macOS-specific implementation details for Muster.

## Table of Contents

- [System Integration](#system-integration)
- [Package Distribution](#package-distribution)
- [Keychain](#keychain)
- [File System](#file-system)
- [Network](#network)
- [Security](#security)

## System Integration

### LaunchAgent (User Space)

For running the application when the user logs in (per-user daemon).

**File:** `packaging/macos/LaunchAgents/com.muster.app.plist`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.app</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>/Applications/Muster.app/Contents/MacOS/Muster</string>
    </array>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    
    <key>ThrottleInterval</key>
    <integer>60</integer>
    
    <key>StandardOutPath</key>
    <string>/Users/$USER/Library/Logs/com.muster.app/stdout.log</string>
    
    <key>StandardErrorPath</key>
    <string>/Users/$USER/Library/Logs/com.muster.app/stderr.log</string>
    
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>HOME</key>
        <string>/Users/$USER</string>
    </dict>
</dict>
</plist>
```

**Installation Script:**

```bash
#!/bin/bash
# install-launchagent.sh

LAUNCH_AGENT="com.muster.app.plist"
LAUNCH_AGENT_DIR="$HOME/Library/LaunchAgents"
LAUNCH_AGENT_PATH="$LAUNCH_AGENT_DIR/$LAUNCH_AGENT"

# Create LaunchAgents directory if needed
mkdir -p "$LAUNCH_AGENT_DIR"

# Install the plist
cp "packaging/macos/LaunchAgents/$LAUNCH_AGENT" "$LAUNCH_AGENT_PATH"

# Load the agent
launchctl load "$LAUNCH_AGENT_PATH"

# Start the agent
launchctl start com.muster.app

echo "LaunchAgent installed and started"
```

### LaunchDaemon (System Space)

For system-wide background processes (requires root).

**File:** `packaging/macos/LaunchDaemons/com.muster.daemon.plist`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.daemon</string>
    
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/libexec/musterd</string>
    </array>
    
    <key>RunAtLoad</key>
    <true/>
    
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
        <key>Crashed</key>
        <true/>
    </dict>
    
    <key>ThrottleInterval</key>
    <integer>10</integer>
    
    <key>UserName</key>
    <string>_muster</string>
    
    <key>GroupName</key>
    <string>_muster</string>
    
    <key>StandardOutPath</key>
    <string>/var/log/muster/stdout.log</string>
    
    <key>StandardErrorPath</key>
    <string>/var/log/muster/stderr.log</string>
    
    <key>HardResourceLimits</key>
    <dict>
        <key>NumberOfFiles</key>
        <integer>4096</integer>
    </dict>
    
    <key>SoftResourceLimits</key>
    <dict>
        <key>NumberOfFiles</key>
        <integer>4096</integer>
    </dict>
</dict>
</plist>
```

**Installation Script:**

```bash
#!/bin/bash
# install-launchdaemon.sh
# Must be run as root

LAUNCH_DAEMON="com.muster.daemon.plist"
LAUNCH_DAEMON_DIR="/Library/LaunchDaemons"
LAUNCH_DAEMON_PATH="$LAUNCH_DAEMON_DIR/$LAUNCH_DAEMON"

# Create log directory
mkdir -p /var/log/muster

# Create muster user and group
if ! id "_muster" &>/dev/null; then
    dscl . -create /Users/_muster
    dscl . -create /Users/_muster UserShell /usr/bin/false
    dscl . -create /Users/_muster RealName "Muster Daemon"
    dscl . -create /Users/_muster UniqueID "$(dscl . -list /Users UniqueID | awk '{print $2}' | sort -n | tail -1 | xargs -I {} echo {} + 1 | bc)"
    dscl . -create /Users/_muster PrimaryGroupID 20
fi

# Install the plist
cp "packaging/macos/LaunchDaemons/$LAUNCH_DAEMON" "$LAUNCH_DAEMON_PATH"
chmod 644 "$LAUNCH_DAEMON_PATH"
chown root:wheel "$LAUNCH_DAEMON_PATH"

# Load the daemon
launchctl load "$LAUNCH_DAEMON_PATH"

# Start the daemon
launchctl start com.muster.daemon

echo "LaunchDaemon installed and started"
```

### Info.plist

**File:** `Muster/Resources/Info.plist`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- Required Keys -->
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    
    <key>CFBundleDisplayName</key>
    <string>Muster</string>
    
    <key>CFBundleExecutable</key>
    <string>Muster</string>
    
    <key>CFBundleIdentifier</key>
    <string>com.muster.app</string>
    
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    
    <key>CFBundleName</key>
    <string>Muster</string>
    
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    
    <key>CFBundleVersion</key>
    <string>100</string>
    
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    
    <!-- App Icon -->
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    
    <key>CFBundleIconName</key>
    <string>AppIcon</string>
    
    <!-- Document Types -->
    <key>CFBundleDocumentTypes</key>
    <array>
        <dict>
            <key>CFBundleTypeName</key>
            <string>Muster Document</string>
            <key>CFBundleTypeRole</key>
            <string>Editor</string>
            <key>CFBundleTypeIconFile</key>
            <string>DocumentIcon</string>
            <key>CFBundleTypeExtensions</key>
            <array>
                <string>muster</string>
            </array>
            <key>CFBundleTypeOSTypes</key>
            <array>
                <string>MUST</string>
            </array>
            <key>LSItemContentTypes</key>
            <array>
                <string>com.muster.document</string>
            </array>
        </dict>
    </array>
    
    <!-- Exported UTIs -->
    <key>UTExportedTypeDeclarations</key>
    <array>
        <dict>
            <key>UTTypeIdentifier</key>
            <string>com.muster.document</string>
            <key>UTTypeDescription</key>
            <string>Muster Document</string>
            <key>UTTypeConformsTo</key>
            <array>
                <string>public.data</string>
                <string>public.content</string>
            </array>
            <key>UTTypeTagSpecification</key>
            <dict>
                <key>public.filename-extension</key>
                <array>
                    <string>muster</string>
                </array>
                <key>public.mime-type</key>
                <array>
                    <string>application/x-muster</string>
                </array>
            </dict>
        </dict>
    </array>
    
    <!-- URL Schemes -->
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLName</key>
            <string>com.muster.url</string>
            <key>CFBundleURLSchemes</key>
            <array>
                <string>muster</string>
            </array>
        </dict>
    </array>
    
    <!-- Background Execution -->
    <key>LSBackgroundOnly</key>
    <false/>
    
    <key>LSUIElement</key>
    <false/>
    
    <key>LSMultipleInstancesProhibited</key>
    <true/>
    
    <!-- Security -->
    <key>NSAppleEventsUsageDescription</key>
    <string>Muster needs to control other applications to integrate with your workflow.</string>
    
    <key>NSAppleScriptEnabled</key>
    <true/>
    
    <key>NSCameraUsageDescription</key>
    <string>Muster does not use the camera.</string>
    
    <key>NSMicrophoneUsageDescription</key>
    <string>Muster does not use the microphone.</string>
    
    <key>NSLocationWhenInUseUsageDescription</key>
    <string>Muster does not use location services.</string>
    
    <key>NSBluetoothAlwaysUsageDescription</key>
    <string>Muster uses Bluetooth for device discovery.</string>
    
    <key>NSLocalNetworkUsageDescription</key>
    <string>Muster uses the local network to discover and connect to other devices.</string>
    
    <!-- App Transport Security -->
    <key>NSAppTransportSecurity</key>
    <dict>
        <key>NSAllowsArbitraryLoads</key>
        <false/>
        <key>NSExceptionDomains</key>
        <dict>
            <key>localhost</key>
            <dict>
                <key>NSExceptionAllowsInsecureHTTPLoads</key>
                <true/>
                <key>NSExceptionMinimumTLSVersion</key>
                <string>TLSv1.2</string>
            </dict>
        </dict>
    </dict>
    
    <!-- User Interface -->
    <key>NSRequiresAquaSystemAppearance</key>
    <false/>
    
    <key>NSHighResolutionCapable</key>
    <true/>
    
    <!-- Services -->
    <key>NSServices</key>
    <array>
        <dict>
            <key>NSMenuItem</key>
            <dict>
                <key>default</key>
                <string>Open with Muster</string>
            </dict>
            <key>NSMessage</key>
            <string>openFile</string>
            <key>NSRequiredContext</key>
            <dict>
                <key>NSTextContent</key>
                <string>FilePath</string>
            </dict>
            <key>NSSendTypes</key>
            <array>
                <string>NSStringPboardType</string>
            </array>
        </dict>
    </array>
    
    <!-- Quick Look -->
    <key>QLSupportedContentTypes</key>
    <array>
        <string>com.muster.document</string>
    </array>
</dict>
</plist>
```

## Package Distribution

### Homebrew Formula

**File:** `packaging/homebrew/muster.rb`

```ruby
class Muster < Formula
  desc "Powerful connectivity and deployment tool"
  homepage "https://muster.io"
  url "https://github.com/muster/muster/archive/v1.0.0.tar.gz"
  sha256 "INSERT_SHA256_HERE"
  license "MIT"
  head "https://github.com/muster/muster.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos => :monterey

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/muster"
    
    # Install LaunchAgent
    (prefix/"com.muster.app.plist").write launchagent_plist
    
    # Create directories
    (var/"log/muster").mkpath
    (etc/"muster").mkpath
  end

  def post_install
    # Load LaunchAgent
    system "launchctl", "load", "-w", "#{prefix}/com.muster.app.plist"
  end

  def caveats
    <<~EOS
      Muster has been installed. The service will start automatically.
      
      To start/stop manually:
        brew services start muster
        brew services stop muster
      
      Configuration: #{etc}/muster/config.yaml
      Logs: #{var}/log/muster
    EOS
  end

  service do
    run [opt_bin/"muster", "daemon"]
    keep_alive true
    error_log_path var/"log/muster/error.log"
    log_path var/"log/muster/output.log"
  end

  test do
    system "#{bin}/muster", "--version"
  end

  private

  def launchagent_plist
    <<~EOS
      <?xml version="1.0" encoding="UTF-8"?>
      <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
      <plist version="1.0">
      <dict>
        <key>Label</key>
        <string>#{plist_name}</string>
        <key>ProgramArguments</key>
        <array>
          <string>#{opt_bin}/muster</string>
          <string>daemon</string>
        </array>
        <key>RunAtLoad</key>
        <true/>
        <key>KeepAlive</key>
        <true/>
      </dict>
      </plist>
    EOS
  end
end
```

### PKG Installer

**Build Script:** `packaging/macos/build-pkg.sh`

```bash
#!/bin/bash
set -e

# Configuration
APP_NAME="Muster"
APP_BUNDLE="${APP_NAME}.app"
VERSION="1.0.0"
BUILD_DIR="build"
PKG_DIR="${BUILD_DIR}/pkg"
SCRIPTS_DIR="packaging/macos/scripts"
RESOURCES_DIR="packaging/macos/resources"
IDENTIFIER="com.muster.app"
CERTIFICATE_NAME="Developer ID Installer: YOUR_NAME (TEAM_ID)"

# Clean and create directories
rm -rf "${PKG_DIR}"
mkdir -p "${PKG_DIR}/Applications"
mkdir -p "${PKG_DIR}/usr/local/bin"
mkdir -p "${PKG_DIR}/Library/LaunchAgents"
mkdir -p "${SCRIPTS_DIR}"

# Copy application
cp -R "${BUILD_DIR}/${APP_BUNDLE}" "${PKG_DIR}/Applications/"

# Create component package
pkgbuild \
    --root "${PKG_DIR}" \
    --identifier "${IDENTIFIER}" \
    --version "${VERSION}" \
    --scripts "${SCRIPTS_DIR}" \
    --install-location "/" \
    "${BUILD_DIR}/${APP_NAME}-component.pkg"

# Create distribution XML
cat > "${BUILD_DIR}/distribution.xml" << EOF
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="1">
    <title>Muster ${VERSION}</title>
    <organization>com.muster</organization>
    <domains enable_localSystem="true"/>
    <options customize="never" require-scripts="true" rootVolumeOnly="true" />
    
    <welcome file="welcome.rtf"/>
    <license file="license.rtf"/>
    <readme file="readme.rtf"/>
    
    <pkg-ref id="${IDENTIFIER}">
        <pkg-ref id="${IDENTIFIER}"/>
    </pkg-ref>
    
    <choices-outline>
        <line choice="default">
            <line choice="${IDENTIFIER}"/>
        </line>
    </choices-outline>
    
    <choice id="default"/>
    <choice id="${IDENTIFIER}" visible="false">
        <pkg-ref id="${IDENTIFIER}"/>
    </choice>
    
    <pkg-ref id="${IDENTIFIER}" version="${VERSION}" onConclusion="none">${APP_NAME}-component.pkg</pkg-ref>
</installer-gui-script>
EOF

# Build final package
productbuild \
    --distribution "${BUILD_DIR}/distribution.xml" \
    --resources "${RESOURCES_DIR}" \
    --package-path "${BUILD_DIR}" \
    --sign "${CERTIFICATE_NAME}" \
    "${BUILD_DIR}/${APP_NAME}-${VERSION}.pkg"

echo "PKG created: ${BUILD_DIR}/${APP_NAME}-${VERSION}.pkg"
```

**Preinstall Script:** `packaging/macos/scripts/preinstall`

```bash
#!/bin/bash

# Check minimum macOS version
MIN_VERSION="12.0"
CURRENT_VERSION=$(sw_vers -productVersion)

if ! printf '%s\n%s\n' "$MIN_VERSION" "$CURRENT_VERSION" | sort -V -C; then
    echo "Error: Muster requires macOS $MIN_VERSION or later"
    exit 1
fi

# Check for conflicting processes
if pgrep -x "Muster" > /dev/null; then
    echo "Stopping existing Muster process..."
    pkill -x "Muster"
    sleep 2
fi

# Unload old LaunchAgent if exists
if launchctl list | grep -q "com.muster.app"; then
    launchctl unload ~/Library/LaunchAgents/com.muster.app.plist 2>/dev/null || true
fi

exit 0
```

**Postinstall Script:** `packaging/macos/scripts/postinstall`

```bash
#!/bin/bash

# Create log directory
mkdir -p ~/Library/Logs/com.muster.app

# Set permissions
chmod +x "/Applications/Muster.app/Contents/MacOS/Muster"

# Load LaunchAgent if it exists
if [ -f ~/Library/LaunchAgents/com.muster.app.plist ]; then
    launchctl load ~/Library/LaunchAgents/com.muster.app.plist
fi

# Register file types
/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister -f "/Applications/Muster.app"

exit 0
```

### DMG Creation

**Build Script:** `packaging/macos/build-dmg.sh`

```bash
#!/bin/bash
set -e

# Configuration
APP_NAME="Muster"
APP_BUNDLE="${APP_NAME}.app"
VERSION="1.0.0"
BUILD_DIR="build"
DMG_NAME="${APP_NAME}-${VERSION}"
DMG_TEMP="${BUILD_DIR}/${DMG_NAME}-temp.dmg"
DMG_FINAL="${BUILD_DIR}/${DMG_NAME}.dmg"
DMG_DIR="${BUILD_DIR}/dmg"
BACKGROUND_IMG="packaging/macos/dmg-background.png"

# Clean
rm -rf "${DMG_DIR}"
mkdir -p "${DMG_DIR}"

# Copy app
cp -R "${BUILD_DIR}/${APP_BUNDLE}" "${DMG_DIR}/"

# Create Applications symlink
ln -s /Applications "${DMG_DIR}/Applications"

# Calculate size
SIZE=$(du -sh "${DMG_DIR}" | cut -f1)
SIZE_MB=$(du -sm "${DMG_DIR}" | cut -f1)
SIZE_MB=$((SIZE_MB + 20))  # Add padding

# Create temporary DMG
hdiutil create \
    -srcfolder "${DMG_DIR}" \
    -volname "${APP_NAME}" \
    -fs HFS+ \
    -fsargs "-c c=64,a=16,e=16" \
    -format UDRW \
    -size "${SIZE_MB}m" \
    "${DMG_TEMP}"

# Mount DMG
DEVICE=$(hdiutil attach -readwrite -noverify -noautoopen "${DMG_TEMP}" | grep "Apple_partition_scheme" | awk '{print $1}')
VOLUME="/Volumes/${APP_NAME}"

# Wait for mount
sleep 2

# Set background image (if provided)
if [ -f "${BACKGROUND_IMG}" ]; then
    mkdir -p "${VOLUME}/.background"
    cp "${BACKGROUND_IMG}" "${VOLUME}/.background/background.png"
    
    # Create .DS_Store with layout
    osascript << EOF
        tell application "Finder"
            tell disk "${APP_NAME}"
                open
                set current view of container window to icon view
                set toolbar visible of container window to false
                set statusbar visible of container window to false
                set the bounds of container window to {400, 100, 885, 430}
                set viewOptions to icon view options of container window
                set arrangement of viewOptions to not arranged
                set icon size of viewOptions to 72
                set background picture of viewOptions to file ".background:background.png"
                set position of item "${APP_BUNDLE}" of container window to {110, 130}
                set position of item "Applications" of container window to {375, 130}
                close
            end tell
        end tell
EOF
fi

# Set icon positions
SetFile -a C "${VOLUME}"

# Sync and unmount
sync
hdiutil detach "${DEVICE}"

# Convert to compressed read-only DMG
hdiutil convert "${DMG_TEMP}" \
    -format UDZO \
    -imagekey zlib-level=9 \
    -o "${DMG_FINAL}"

# Sign DMG
codesign --sign "Developer ID Application: YOUR_NAME" \
    --timestamp \
    "${DMG_FINAL}"

# Clean up
rm -f "${DMG_TEMP}"
rm -rf "${DMG_DIR}"

echo "DMG created: ${DMG_FINAL}"
echo "Size: $(du -h "${DMG_FINAL}" | cut -f1)"
```

### Notarization

**Notarization Script:** `packaging/macos/notarize.sh`

```bash
#!/bin/bash
set -e

# Configuration
BUNDLE_ID="com.muster.app"
APPLE_ID="your.email@example.com"
APPLE_PASSWORD="@keychain:AC_PASSWORD"  # App-specific password in keychain
TEAM_ID="YOUR_TEAM_ID"
DMG_PATH="build/Muster-1.0.0.dmg"

# Step 1: Submit for notarization
echo "Submitting for notarization..."
UPLOAD_RESULT=$(xcrun notarytool submit "${DMG_PATH}" \
    --apple-id "${APPLE_ID}" \
    --password "${APPLE_PASSWORD}" \
    --team-id "${TEAM_ID}" \
    --wait)

# Extract submission ID
SUBMISSION_ID=$(echo "${UPLOAD_RESULT}" | grep "id:" | head -1 | awk '{print $2}')

# Step 2: Check status
echo "Waiting for notarization..."
while true; do
    STATUS=$(xcrun notarytool info "${SUBMISSION_ID}" \
        --apple-id "${APPLE_ID}" \
        --password "${APPLE_PASSWORD}" \
        --team-id "${TEAM_ID}" \
        --output-format json | jq -r '.status')
    
    if [ "${STATUS}" == "Accepted" ]; then
        echo "Notarization successful!"
        break
    elif [ "${STATUS}" == "Invalid" ] || [ "${STATUS}" == "Rejected" ]; then
        echo "Notarization failed!"
        xcrun notarytool log "${SUBMISSION_ID}" \
            --apple-id "${APPLE_ID}" \
            --password "${APPLE_PASSWORD}" \
            --team-id "${TEAM_ID}"
        exit 1
    fi
    
    echo "Status: ${STATUS}, waiting..."
    sleep 30
done

# Step 3: Staple the ticket
echo "Stapling notarization ticket..."
xcrun stapler staple "${DMG_PATH}"

# Verify
echo "Verifying notarization..."
spctl -a -t open --context context:primary-signature -v "${DMG_PATH}"

echo "Notarization complete!"
```

**Notarization for ZIP/Archive:**

```bash
#!/bin/bash

# For ZIP files, submit the contents separately
APP_PATH="build/Muster.app"
ZIP_PATH="build/Muster.zip"

# Create ZIP
ditto -c -k --keepParent "${APP_PATH}" "${ZIP_PATH}"

# Notarize ZIP
xcrun notarytool submit "${ZIP_PATH}" \
    --apple-id "${APPLE_ID}" \
    --password "${APPLE_PASSWORD}" \
    --team-id "${TEAM_ID}" \
    --wait

# After notarization, staple the app directly
codesign --deep --force --verify --verbose --sign "Developer ID Application: YOUR_NAME" "${APP_PATH}"
xcrun stapler staple "${APP_PATH}"

# Create final ZIP with stapled app
ditto -c -k --keepParent "${APP_PATH}" "${ZIP_PATH}"
```

## Keychain

### Security Framework Integration

**Swift wrapper for Keychain access:**

```swift
// Sources/Muster/Security/Keychain.swift
import Foundation
import Security

enum KeychainError: Error {
    case itemNotFound
    case duplicateItem
    case invalidStatus(OSStatus)
    case invalidData
}

class Keychain {
    
    static let shared = Keychain()
    
    private let service = "com.muster.app"
    private let accessGroup = "TEAM_ID.com.muster.shared"
    
    // MARK: - Save
    
    func save(password: String, account: String) throws {
        guard let passwordData = password.data(using: .utf8) else {
            throw KeychainError.invalidData
        }
        
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: passwordData,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
            kSecAttrAccessGroup as String: accessGroup,
            kSecUseDataProtectionKeychain as String: true
        ]
        
        let status = SecItemAdd(query as CFDictionary, nil)
        
        if status == errSecDuplicateItem {
            // Update existing item
            let updateQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account
            ]
            
            let attributesToUpdate: [String: Any] = [
                kSecValueData as String: passwordData
            ]
            
            let updateStatus = SecItemUpdate(
                updateQuery as CFDictionary,
                attributesToUpdate as CFDictionary
            )
            
            guard updateStatus == errSecSuccess else {
                throw KeychainError.invalidStatus(updateStatus)
            }
        } else if status != errSecSuccess {
            throw KeychainError.invalidStatus(status)
        }
    }
    
    // MARK: - Retrieve
    
    func retrieve(account: String) throws -> String {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        
        guard status == errSecSuccess else {
            if status == errSecItemNotFound {
                throw KeychainError.itemNotFound
            }
            throw KeychainError.invalidStatus(status)
        }
        
        guard let passwordData = result as? Data,
              let password = String(data: passwordData, encoding: .utf8) else {
            throw KeychainError.invalidData
        }
        
        return password
    }
    
    // MARK: - Delete
    
    func delete(account: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        
        let status = SecItemDelete(query as CFDictionary)
        
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError.invalidStatus(status)
        }
    }
    
    // MARK: - Certificate Access
    
    func importCertificate(data: Data, password: String?) throws {
        var importResult: CFArray?
        let options: [String: Any] = [
            kSecImportExportPassphrase as String: password ?? ""
        ]
        
        let status = SecPKCS12Import(
            data as CFData,
            options as CFDictionary,
            &importResult
        )
        
        guard status == errSecSuccess else {
            throw KeychainError.invalidStatus(status)
        }
    }
}
```

### Keychain Access Groups

**Entitlements Configuration:**

```xml
<!-- Muster.entitlements -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- Enable Data Protection keychain -->
    <key>keychain-access-groups</key>
    <array>
        <string>$(AppIdentifierPrefix)com.muster.shared</string>
    </array>
    
    <!-- Keychain Sharing -->
    <key>com.apple.security.application-groups</key>
    <array>
        <string>group.com.muster.app</string>
    </array>
</dict>
</plist>
```

**Provisioning Profile Setup:**

```bash
#!/bin/bash
# Configure keychain access groups in Apple Developer Portal

# 1. Enable "Keychain Sharing" in App ID
# 2. Create an App Group: group.com.muster.app
# 3. Assign the App Group to your App ID
# 4. Regenerate provisioning profiles

# Verify keychain access
echo "Testing keychain access..."
security find-identity -v -p codesigning
```

## File System

### Standard Directories

```swift
// Sources/Muster/FileSystem/Directories.swift
import Foundation

struct AppDirectories {
    
    static let shared = AppDirectories()
    
    // MARK: - Application Support
    
    var applicationSupport: URL {
        let urls = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        )
        return urls[0].appendingPathComponent("Muster", isDirectory: true)
    }
    
    // MARK: - Caches
    
    var caches: URL {
        let urls = FileManager.default.urls(
            for: .cachesDirectory,
            in: .userDomainMask
        )
        return urls[0].appendingPathComponent("Muster", isDirectory: true)
    }
    
    // MARK: - Logs
    
    var logs: URL {
        let urls = FileManager.default.urls(
            for: .libraryDirectory,
            in: .userDomainMask
        )
        return urls[0]
            .appendingPathComponent("Logs", isDirectory: true)
            .appendingPathComponent("com.muster.app", isDirectory: true)
    }
    
    // MARK: - Preferences
    
    var preferences: URL {
        let urls = FileManager.default.urls(
            for: .libraryDirectory,
            in: .userDomainMask
        )
        return urls[0].appendingPathComponent("Preferences", isDirectory: true)
    }
    
    // MARK: - Temporary
    
    var temporary: URL {
        FileManager.default.temporaryDirectory
            .appendingPathComponent("com.muster.app", isDirectory: true)
    }
    
    // MARK: - Configuration
    
    var config: URL {
        applicationSupport.appendingPathComponent("config", isDirectory: true)
    }
    
    // MARK: - Database
    
    var database: URL {
        applicationSupport.appendingPathComponent("data", isDirectory: true)
    }
    
    // MARK: - Create Directories
    
    func createDirectories() throws {
        let directories = [
            applicationSupport,
            caches,
            logs,
            config,
            database,
            temporary
        ]
        
        for directory in directories {
            try FileManager.default.createDirectory(
                at: directory,
                withIntermediateDirectories: true,
                attributes: nil
            )
        }
    }
}
```

### Sandboxing

**Entitlements for Sandboxed App:**

```xml
<!-- Muster.entitlements -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- Enable App Sandbox -->
    <key>com.apple.security.app-sandbox</key>
    <true/>
    
    <!-- Network Access -->
    <key>com.apple.security.network.client</key>
    <true/>
    <key>com.apple.security.network.server</key>
    <true/>
    
    <!-- File Access -->
    <key>com.apple.security.files.user-selected.read-write</key>
    <true/>
    
    <!-- Downloads folder -->
    <key>com.apple.security.files.downloads.read-write</key>
    <true/>
    
    <!-- USB Devices -->
    <key>com.apple.security.device.usb</key>
    <true/>
    
    <!-- Bluetooth -->
    <key>com.apple.security.device.bluetooth</key>
    <true/>
    
    <!-- Printing -->
    <key>com.apple.security.print</key>
    <true/>
    
    <!-- Temporary Exception for Home Directory -->
    <key>com.apple.security.temporary-exception.files.home-relative-path.read-write</key>
    <array>
        <string>/Library/Application Support/Muster/</string>
    </array>
    
    <!-- Temporary Exception for Network -->
    <key>com.apple.security.temporary-exception.mach-lookup.global-name</key>
    <array>
        <string>com.muster.daemon</string>
    </array>
</dict>
</plist>
```

**Bookmarker for Sandboxed File Access:**

```swift
// Sources/Muster/FileSystem/Bookmarker.swift
import Foundation

class Bookmarker {
    
    static let shared = Bookmarker()
    
    private let userDefaults = UserDefaults.standard
    private let bookmarkKey = "securityScopedBookmarks"
    
    // Save bookmark for a URL
    func saveBookmark(for url: URL) throws {
        let bookmarkData = try url.bookmarkData(
            options: .withSecurityScope,
            includingResourceValuesForKeys: nil,
            relativeTo: nil
        )
        
        var bookmarks = userDefaults.dictionary(forKey: bookmarkKey) ?? [:]
        bookmarks[url.path] = bookmarkData
        userDefaults.set(bookmarks, forKey: bookmarkKey)
    }
    
    // Resolve bookmark and start accessing
    func resolveBookmark(for path: String) -> URL? {
        guard let bookmarks = userDefaults.dictionary(forKey: bookmarkKey),
              let bookmarkData = bookmarks[path] as? Data else {
            return nil
        }
        
        var isStale = false
        do {
            let url = try URL(
                resolvingBookmarkData: bookmarkData,
                options: .withSecurityScope,
                relativeTo: nil,
                bookmarkDataIsStale: &isStale
            )
            
            if isStale {
                // Re-create bookmark if stale
                _ = try? saveBookmark(for: url)
            }
            
            _ = url.startAccessingSecurityScopedResource()
            return url
        } catch {
            return nil
        }
    }
    
    // Stop accessing security-scoped resource
    func stopAccessing(_ url: URL) {
        url.stopAccessingSecurityScopedResource()
    }
}
```

## Network

### System Proxy Support

```swift
// Sources/Muster/Network/SystemProxy.swift
import Foundation
import SystemConfiguration

class SystemProxy {
    
    static let shared = SystemProxy()
    
    struct ProxySettings {
        let enabled: Bool
        let host: String?
        let port: Int?
        let username: String?
        let password: String?
    }
    
    // Get current system proxy settings
    func getProxySettings() -> ProxySettings {
        let proxySettings = CFNetworkCopySystemProxySettings()?.takeRetainedValue() as? [String: Any]
        
        let httpProxy = proxySettings?[kCFNetworkProxiesHTTPEnable as String] as? Int == 1
        let host = proxySettings?[kCFNetworkProxiesHTTPProxy as String] as? String
        let port = proxySettings?[kCFNetworkProxiesHTTPPort as String] as? Int
        
        return ProxySettings(
            enabled: httpProxy,
            host: host,
            port: port,
            username: nil,
            password: nil
        )
    }
    
    // Configure URLSession to use system proxy
    func configureWithSystemProxy(_ sessionConfiguration: URLSessionConfiguration) {
        let proxySettings = getProxySettings()
        
        guard proxySettings.enabled,
              let host = proxySettings.host,
              let port = proxySettings.port else {
            return
        }
        
        let proxyConfig = [
            kCFNetworkProxiesHTTPEnable: true,
            kCFNetworkProxiesHTTPProxy: host,
            kCFNetworkProxiesHTTPPort: port,
            kCFNetworkProxiesHTTPSEnable: true,
            kCFNetworkProxiesHTTPSProxy: host,
            kCFNetworkProxiesHTTPSPort: port
        ] as [AnyHashable: Any]
        
        sessionConfiguration.connectionProxyDictionary = proxyConfig
    }
}
```

### NetworkExtension (Optional)

For packet tunneling or content filtering:

```swift
// Network Extension capability required
// Add to entitlements:
// com.apple.developer.networking.networkextension

import NetworkExtension

class VPNManager {
    
    static let shared = VPNManager()
    private let manager = NEVPNManager.shared()
    
    func configureVPN() {
        manager.loadFromPreferences { error in
            guard error == nil else {
                print("Failed to load preferences: \(error!)")
                return
            }
            
            let protocolConfig = NEVPNProtocolIKEv2()
            protocolConfig.serverAddress = "vpn.muster.io"
            protocolConfig.remoteIdentifier = "vpn.muster.io"
            protocolConfig.localIdentifier = "client"
            protocolConfig.authenticationMethod = .none
            
            self.manager.protocolConfiguration = protocolConfig
            self.manager.localizedDescription = "Muster VPN"
            self.manager.isEnabled = true
            
            self.manager.saveToPreferences { error in
                if let error = error {
                    print("Failed to save: \(error)")
                }
            }
        }
    }
}
```

## Security

### Code Signing Configuration

**Entitlements:** `Muster.entitlements`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- App Sandbox -->
    <key>com.apple.security.app-sandbox</key>
    <true/>
    
    <!-- Hardened Runtime -->
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    <key>com.apple.security.cs.allow-unsigned-executable-memory</key>
    <true/>
    <key>com.apple.security.cs.disable-library-validation</key>
    <false/>
    <key>com.apple.security.cs.debugger</key>
    <true/>
    <key>com.apple.security.automation.apple-events</key>
    <true/>
    
    <!-- Network -->
    <key>com.apple.security.network.client</key>
    <true/>
    <key>com.apple.security.network.server</key>
    <true/>
    
    <!-- File Access -->
    <key>com.apple.security.files.user-selected.read-write</key>
    <true/>
    <key>com.apple.security.files.downloads.read-write</key>
    <true/>
    
    <!-- Keychain -->
    <key>keychain-access-groups</key>
    <array>
        <string>$(AppIdentifierPrefix)com.muster.shared</string>
    </array>
    
    <!-- App Groups -->
    <key>com.apple.security.application-groups</key>
    <array>
        <string>group.com.muster.app</string>
    </array>
    
    <!-- Hardware -->
    <key>com.apple.security.device.usb</key>
    <true/>
    <key>com.apple.security.device.bluetooth</key>
    <true/>
    
    <!-- Temporary Exceptions (if needed) -->
    <key>com.apple.security.temporary-exception.files.absolute-path.read-write</key>
    <array>
        <string>/usr/local/var/muster/</string>
    </array>
</dict>
</plist>
```

**Signing Script:** `scripts/sign.sh`

```bash
#!/bin/bash
set -e

APP_NAME="Muster"
APP_PATH="build/${APP_NAME}.app"
ENTITLEMENTS="Muster.entitlements"
SIGNING_IDENTITY="Developer ID Application: YOUR_NAME (TEAM_ID)"

# Sign embedded frameworks
find "${APP_PATH}/Contents/Frameworks" -name "*.framework" -type d | while read -r framework; do
    codesign --force --sign "${SIGNING_IDENTITY}" \
        --options runtime \
        --timestamp \
        "${framework}/Versions/Current/"*
done

# Sign plugins
find "${APP_PATH}/Contents/PlugIns" -name "*.appex" -type d | while read -r plugin; do
    codesign --force --sign "${SIGNING_IDENTITY}" \
        --entitlements "${ENTITLEMENTS}" \
        --options runtime \
        --timestamp \
        "${plugin}"
done

# Sign main executable
codesign --force --sign "${SIGNING_IDENTITY}" \
    --entitlements "${ENTITLEMENTS}" \
    --options runtime \
    --timestamp \
    "${APP_PATH}"

# Verify
echo "Verifying signature..."
codesign --verify --deep --strict --verbose=2 "${APP_PATH}"
spctl -a -t exec -vv "${APP_PATH}"

echo "Signing complete!"
```

### Hardened Runtime

**Requirements:**

```
# codesign requirements
designated => anchor apple generic and identifier "com.muster.app" and (certificate leaf[field.1.2.840.113635.100.6.1.9] /* exists */ or certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = "TEAM_ID")
```

**Debug Hardened Runtime Issues:**

```bash
#!/bin/bash
# Enable crash reporting for hardened runtime

defaults write com.muster.app NSApplicationCrashOnExceptions -bool true

# Check for library validation failures
# Look for "code signature invalid" in Console.app

# Test with library validation disabled (development only)
codesign --force --sign "${SIGNING_IDENTITY}" \
    --entitlements Muster.entitlements \
    --options runtime \
    --timestamp \
    --disable-library-validation \
    "${APP_PATH}"
```

### Entitlements Reference

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- ============================================ -->
    <!-- Core Hardened Runtime Entitlements -->
    <!-- ============================================ -->
    
    <!-- Allow Just-In-Time (JIT) compilation (required for Electron/Node.js) -->
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    
    <!-- Allow unsigned executable memory -->
    <key>com.apple.security.cs.allow-unsigned-executable-memory</key>
    <true/>
    
    <!-- Disable library validation (use with caution) -->
    <key>com.apple.security.cs.disable-library-validation</key>
    <false/>
    
    <!-- Allow debugger attachment -->
    <key>com.apple.security.cs.debugger</key>
    <true/>
    
    <!-- Allow dyld environment variables -->
    <key>com.apple.security.cs.allow-dyld-environment-variables</key>
    <false/>
    
    <!-- Disable team identifier validation -->
    <key>com.apple.security.cs.disable-library-validation</key>
    <false/>
    
    <!-- ============================================ -->
    <!-- App Sandbox Entitlements -->
    <!-- ============================================ -->
    
    <!-- Enable App Sandbox -->
    <key>com.apple.security.app-sandbox</key>
    <true/>
    
    <!-- Inherit App Sandbox (for XPC services) -->
    <key>com.apple.security.inherit</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- File System Access -->
    <!-- ============================================ -->
    
    <!-- User-selected files -->
    <key>com.apple.security.files.user-selected.read-only</key>
    <true/>
    <key>com.apple.security.files.user-selected.read-write</key>
    <true/>
    
    <!-- Downloads folder -->
    <key>com.apple.security.files.downloads.read-only</key>
    <true/>
    <key>com.apple.security.files.downloads.read-write</key>
    <true/>
    
    <!-- Music folder -->
    <key>com.apple.security.assets.music.read-only</key>
    <true/>
    <key>com.apple.security.assets.music.read-write</key>
    <true/>
    
    <!-- Pictures folder -->
    <key>com.apple.security.assets.pictures.read-only</key>
    <true/>
    <key>com.apple.security.assets.pictures.read-write</key>
    <true/>
    
    <!-- Movies folder -->
    <key>com.apple.security.assets.movies.read-only</key>
    <true/>
    <key>com.apple.security.assets.movies.read-write</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- Network Access -->
    <!-- ============================================ -->
    
    <!-- Outbound network connections -->
    <key>com.apple.security.network.client</key>
    <true/>
    
    <!-- Inbound network connections -->
    <key>com.apple.security.network.server</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- Hardware Access -->
    <!-- ============================================ -->
    
    <!-- Camera -->
    <key>com.apple.security.device.camera</key>
    <true/>
    
    <!-- Microphone -->
    <key>com.apple.security.device.microphone</key>
    <true/>
    
    <!-- Bluetooth -->
    <key>com.apple.security.device.bluetooth</key>
    <true/>
    
    <!-- USB -->
    <key>com.apple.security.device.usb</key>
    <true/>
    
    <!-- Printing -->
    <key>com.apple.security.print</key>
    <true/>
    
    <!-- Audio input -->
    <key>com.apple.security.device.audio-input</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- Personal Information -->
    <!-- ============================================ -->
    
    <!-- Address book -->
    <key>com.apple.security.personal-information.addressbook</key>
    <true/>
    
    <!-- Location -->
    <key>com.apple.security.personal-information.location</key>
    <true/>
    
    <!-- Calendar -->
    <key>com.apple.security.personal-information.calendars</key>
    <true/>
    
    <!-- Photos library -->
    <key>com.apple.security.personal-information.photos-library</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- Automation -->
    <!-- ============================================ -->
    
    <!-- Apple Events -->
    <key>com.apple.security.automation.apple-events</key>
    <true/>
    
    <!-- ============================================ -->
    <!-- Keychain -->
    <!-- ============================================ -->
    
    <key>keychain-access-groups</key>
    <array>
        <string>$(AppIdentifierPrefix)com.muster.shared</string>
    </array>
    
    <!-- ============================================ -->
    <!-- App Groups -->
    <!-- ============================================ -->
    
    <key>com.apple.security.application-groups</key>
    <array>
        <string>group.com.muster.app</string>
    </array>
    
    <!-- ============================================ -->
    <!-- Temporary Exceptions -->
    <!-- ============================================ -->
    
    <!-- Absolute path read/write -->
    <key>com.apple.security.temporary-exception.files.absolute-path.read-write</key>
    <array>
        <string>/usr/local/var/muster/</string>
    </array>
    
    <!-- Home-relative path -->
    <key>com.apple.security.temporary-exception.files.home-relative-path.read-write</key>
    <array>
        <string>/Library/Application Support/Muster/</string>
    </array>
    
    <!-- Mach lookup -->
    <key>com.apple.security.temporary-exception.mach-lookup.global-name</key>
    <array>
        <string>com.muster.daemon</string>
    </array>
    
    <!-- Shared preference domain -->
    <key>com.apple.security.temporary-exception.shared-preference.read-write</key>
    <array>
        <string>com.muster.app</string>
    </array>
    
    <!-- iCloud key-value store -->
    <key>com.apple.developer.ubiquity-kvstore-identifier</key>
    <string>$(TeamIdentifierPrefix)$(CFBundleIdentifier)</string>
    
    <!-- iCloud containers -->
    <key>com.apple.developer.icloud-container-identifiers</key>
    <array>
        <string>iCloud.com.muster.app</string>
    </array>
    
    <!-- iCloud services -->
    <key>com.apple.developer.icloud-services</key>
    <array>
        <string>CloudKit</string>
        <string>CloudDocuments</string>
    </array>
</dict>
</plist>
```

---

## Quick Reference

### File Locations

| Purpose | Path |
|---------|------|
| Application Bundle | `/Applications/Muster.app` |
| User Application Support | `~/Library/Application Support/Muster/` |
| System Application Support | `/Library/Application Support/Muster/` |
| User Caches | `~/Library/Caches/com.muster.app/` |
| User Preferences | `~/Library/Preferences/com.muster.app.plist` |
| User Logs | `~/Library/Logs/com.muster.app/` |
| LaunchAgent | `~/Library/LaunchAgents/com.muster.app.plist` |
| LaunchDaemon | `/Library/LaunchDaemons/com.muster.daemon.plist` |

### Commands

```bash
# Load/unload LaunchAgent
launchctl load ~/Library/LaunchAgents/com.muster.app.plist
launchctl unload ~/Library/LaunchAgents/com.muster.app.plist

# Check code signing
codesign -dvv /Applications/Muster.app
codesign --verify --deep --strict /Applications/Muster.app

# Check notarization
spctl -a -vv /Applications/Muster.app

# View entitlements
codesign -d --entitlements - /Applications/Muster.app

# Reset app preferences
defaults delete com.muster.app

# View logs
log show --predicate 'subsystem == "com.muster.app"' --last 1h
```

### Build Commands

```bash
# Build and sign
xcodebuild -project Muster.xcodeproj -scheme Muster -configuration Release

# Create PKG
./packaging/macos/build-pkg.sh

# Create DMG
./packaging/macos/build-dmg.sh

# Notarize
./packaging/macos/notarize.sh
```
