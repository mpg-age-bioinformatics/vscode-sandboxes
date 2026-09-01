on run argv
  set volumeName to item 1 of argv
  tell application "Finder"
    set targetDisk to disk (volumeName as text)
    tell targetDisk
      open
      tell container window
        set current view to icon view
        set toolbar visible to false
        set statusbar visible to false
        set pathbar visible to false
        set bounds to {120, 120, 700, 460}
      end tell
      tell icon view options of container window
        set arrangement to not arranged
        set icon size to 96
        set text size to 13
      end tell
      set position of item "Bioinformatics Sandbox.app" to {155, 165}
      set position of item "Applications" to {425, 165}
      update without registering applications
      delay 2
      close
    end tell
  end tell
end run
