import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    spacing: 10
    RowLayout {
        Layout.fillWidth: true
        Text { text: "ABOUT & UPDATES"; color: Theme.foreground; font.pixelSize: 12; font.weight: Font.Medium; Layout.fillWidth: true }
        Text { text: "Headroom " + appInfo.applicationVersion; color: Theme.purple; font.pixelSize: 12 }
    }
    RowLayout {
        Layout.fillWidth: true
        ColumnLayout {
            Layout.fillWidth: true; spacing: 4
            Text { text: appInfo.serverVersion ? "Usage server " + appInfo.serverVersion : "Usage server"; color: Theme.foreground; font.pixelSize: 12 }
            Text { text: appInfo.serverStatus; Layout.fillWidth: true; wrapMode: Text.WordWrap; color: Theme.muted; font.pixelSize: 11; textFormat: Text.PlainText }
        }
        ActionButton { text: appInfo.checkingServer ? "Checking…" : "Check server"; enabled: !appInfo.checkingServer; quiet: true; onClicked: appInfo.refreshServer() }
    }
    Text { text: appInfo.releaseStatus; Layout.fillWidth: true; wrapMode: Text.WordWrap; color: Theme.muted; font.pixelSize: 12; textFormat: Text.PlainText }
    Flow {
        Layout.fillWidth: true; spacing: 8
        ActionButton { text: appInfo.checkingRelease ? "Checking…" : "Check for updates"; enabled: !appInfo.checkingRelease; onClicked: appInfo.checkForUpdates() }
        ActionButton { text: appInfo.linuxDownloadAvailable ? "View Linux release" : "Release notes"; quiet: true; onClicked: appInfo.openReleasePage() }
        ActionButton { text: "Source update guide"; quiet: true; onClicked: appInfo.openInstallGuide() }
    }
}
