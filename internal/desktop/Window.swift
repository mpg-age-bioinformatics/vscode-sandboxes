import AppKit
import WebKit

final class WindowDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandlerWithReply {
    var window: NSWindow!
    var web: WKWebView!
    func applicationDidFinishLaunching(_ notification: Notification) {
        // This helper runs from Contents/Resources rather than Contents/MacOS.
        // Use the enclosing app's icon for its Dock presence as well.
        let executable = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath()
        let bundleURL = executable.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        if let bundle = Bundle(url: bundleURL),
           let iconName = bundle.object(forInfoDictionaryKey: "CFBundleIconFile") as? String,
           let iconURL = bundle.url(forResource: iconName, withExtension: nil),
           let icon = NSImage(contentsOf: iconURL) {
            NSApp.applicationIconImage = icon
        }
        installMenus()
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1000, height: 820), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = CommandLine.arguments[2]
        window.minSize = NSSize(width: 760, height: 620)
        window.center()
        window.delegate = self
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addScriptMessageHandler(self, contentWorld: .page, name: "chooseFolder")
        configuration.userContentController.addScriptMessageHandler(self, contentWorld: .page, name: "exportActivity")
        web = WKWebView(frame: window.contentView!.bounds, configuration: configuration)
        web.autoresizingMask = [.width, .height]
        web.navigationDelegate = self
        web.uiDelegate = self
        window.contentView!.addSubview(web)
        web.load(URLRequest(url: URL(string: CommandLine.arguments[1])!))
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }
    func installMenus() {
        // AppKit routes keyboard equivalents through the menu to WebKit's
        // focused text editor. Keep targets nil for standard responder actions.
        let main = NSMenu()
        let applicationItem = NSMenuItem()
        let application = NSMenu()
        let quit = NSMenuItem(title: "Quit \(CommandLine.arguments[2])", action: #selector(closeApplication(_:)), keyEquivalent: "q")
        quit.target = self
        application.addItem(quit)
        applicationItem.submenu = application
        main.addItem(applicationItem)

        let editItem = NSMenuItem(title: "Edit", action: nil, keyEquivalent: "")
        let edit = NSMenu(title: "Edit")
        edit.addItem(NSMenuItem(title: "Undo", action: Selector(("undo:")), keyEquivalent: "z"))
        let redo = NSMenuItem(title: "Redo", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        edit.addItem(redo)
        edit.addItem(.separator())
        edit.addItem(NSMenuItem(title: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x"))
        edit.addItem(NSMenuItem(title: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c"))
        edit.addItem(NSMenuItem(title: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v"))
        edit.addItem(NSMenuItem(title: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a"))
        editItem.submenu = edit
        main.addItem(editItem)
        NSApp.mainMenu = main
    }
    @objc func closeApplication(_ sender: Any?) {
        window.performClose(sender)
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        if let url = navigationAction.request.url, url.host != "127.0.0.1" {
            if ["http", "https"].contains(url.scheme ?? "") { NSWorkspace.shared.open(url) }
            decisionHandler(.cancel)
        } else { decisionHandler(.allow) }
    }
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let url = navigationAction.request.url, ["http", "https"].contains(url.scheme ?? ""), url.host != "127.0.0.1" {
            NSWorkspace.shared.open(url)
        }
        return nil
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
        guard ["chooseFolder", "exportActivity"].contains(message.name), message.frameInfo.isMainFrame,
              message.frameInfo.securityOrigin.host == "127.0.0.1",
              message.frameInfo.request.url?.absoluteString == CommandLine.arguments[1] else {
            replyHandler(nil, "Folder selection is only available in the startup window.")
            return
        }
        if message.name == "exportActivity" {
            guard let text = message.body as? String else { replyHandler(nil, "Invalid activity export"); return }
            let panel = NSSavePanel()
            panel.title = "Export startup activity"
            panel.nameFieldStringValue = "startup-activity.txt"
            panel.canCreateDirectories = true
            panel.beginSheetModal(for: window) { response in
                guard response == .OK, let url = panel.url else { replyHandler("", nil); return }
                do { try text.write(to: url, atomically: true, encoding: .utf8); replyHandler(url.path, nil) }
                catch { replyHandler(nil, "Could not export startup activity: \(error.localizedDescription)") }
            }
            return
        }
        let panel = NSOpenPanel()
        panel.title = "Project folder"
        panel.message = "Choose or create your sandbox project folder."
        panel.prompt = "Choose folder"
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.canCreateDirectories = true
        panel.allowsMultipleSelection = false
        let proposed = message.body as? String ?? ""
        var directory = URL(fileURLWithPath: proposed.hasPrefix("/") ? proposed : NSHomeDirectory(), isDirectory: true)
        var isDirectory: ObjCBool = false
        while !FileManager.default.fileExists(atPath: directory.path, isDirectory: &isDirectory) || !isDirectory.boolValue {
            let parent = directory.deletingLastPathComponent()
            if parent == directory { break }
            directory = parent
        }
        panel.directoryURL = directory
        panel.beginSheetModal(for: window) { response in
            replyHandler(response == .OK ? (panel.url?.path ?? "") : "", nil)
        }
    }
    func windowShouldClose(_ sender: NSWindow) -> Bool {
        // Startup commands must finish before the controller exits.
        web.evaluateJavaScript("document.body.dataset.running === 'true'") { value, _ in
            if value as? Bool == true {
                let alert = NSAlert(); alert.messageText = "Startup is still running"; alert.informativeText = "Wait for startup to finish before closing this window."; alert.runModal()
            } else { NSApp.terminate(nil) }
        }
        return false
    }
}
let app = NSApplication.shared
let delegate = WindowDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
