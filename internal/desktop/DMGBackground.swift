// Reproducible Finder installation background, with 1x and 2x representations.
import AppKit

let size = NSSize(width: 720, height: 560)
let image = NSImage(size: size)
func color(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat) -> NSColor {
    NSColor(srgbRed: r / 255, green: g / 255, blue: b / 255, alpha: 1)
}
func text(_ value: String, x: CGFloat, y: CGFloat, font: NSFont, color: NSColor, centered: Bool = false) {
    let string = NSAttributedString(string: value, attributes: [.font: font, .foregroundColor: color])
    string.draw(at: NSPoint(x: centered ? x - string.size().width / 2 : x, y: y))
}
for scale in [1, 2] {
    let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: Int(size.width) * scale,
                                 pixelsHigh: Int(size.height) * scale, bitsPerSample: 8,
                                 samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
                                 colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    bitmap.size = size
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
    // NSGraphicsContext honors bitmap.size and scales the 2x representation.
    color(241, 245, 248).setFill()
    NSRect(origin: .zero, size: size).fill()
    color(21, 46, 58).setFill()
    NSRect(x: 0, y: 440, width: 720, height: 120).fill()
    text("Install VS Code Sandboxes", x: 42, y: 495,
         font: .systemFont(ofSize: 27, weight: .semibold), color: .white)
    text("Python · R · General bioinformatics", x: 42, y: 465,
         font: .systemFont(ofSize: 15), color: color(184, 208, 217))

    color(23, 107, 121).setFill()
    let arrow = NSBezierPath()
    arrow.move(to: NSPoint(x: 300, y: 340))
    for point in [NSPoint(x: 394, y: 340), NSPoint(x: 394, y: 361),
                  NSPoint(x: 430, y: 330), NSPoint(x: 394, y: 299),
                  NSPoint(x: 394, y: 320), NSPoint(x: 300, y: 320)] { arrow.line(to: point) }
    arrow.close()
    arrow.fill()
    text("Drag the app to Applications", x: 360, y: 210,
         font: .systemFont(ofSize: 18, weight: .medium), color: color(25, 44, 60), centered: true)
    text("Then open it from Applications to get started.", x: 360, y: 184,
         font: .systemFont(ofSize: 13), color: color(96, 119, 131), centered: true)
    color(218, 227, 232).setFill()
    NSRect(x: 42, y: 159, width: 636, height: 1).fill()
    NSGraphicsContext.restoreGraphicsState()
    image.addRepresentation(bitmap)
}
try image.tiffRepresentation!.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
