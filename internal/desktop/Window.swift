import AppKit
import WebKit

final class WindowDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKUIDelegate {
    var window: NSWindow!
    var web: WKWebView!
    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1000, height: 820), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = CommandLine.arguments[2]
        window.minSize = NSSize(width: 760, height: 620)
        window.center()
        window.delegate = self
        web = WKWebView(frame: window.contentView!.bounds)
        web.autoresizingMask = [.width, .height]
        web.navigationDelegate = self
        web.uiDelegate = self
        window.contentView!.addSubview(web)
        web.load(URLRequest(url: URL(string: CommandLine.arguments[1])!))
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
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
