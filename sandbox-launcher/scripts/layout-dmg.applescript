on run argv
  set volumeName to item 1 of argv
  set backgroundFile to (POSIX file ((item 2 of argv) & "/.background/installation.tiff")) as alias
  tell application "Finder"
    set targetDisk to disk (volumeName as text)
    tell targetDisk
      open
      tell container window
        set current view to icon view
        set toolbar visible to false
        set statusbar visible to false
        set pathbar visible to false
        set bounds to {120, 120, 840, 712}
      end tell
      tell icon view options of container window
        set arrangement to not arranged
        set icon size to 96
        set text size to 12
        set background picture to backgroundFile
      end tell
      set position of item "VS Code Sandboxes.app" to {180, 230}
      set position of item "Applications" to {540, 230}
      set position of item "LICENSE" to {210, 450}
      set position of item "THIRD-PARTY-NOTICES.txt" to {510, 450}
      update without registering applications
      delay 2
      close
    end tell
  end tell
end run
