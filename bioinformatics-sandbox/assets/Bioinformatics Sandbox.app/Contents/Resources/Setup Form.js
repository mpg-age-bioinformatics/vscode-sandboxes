ObjC.import("AppKit");

function label(text, frame) {
  const field = $.NSTextField.labelWithString(text);
  field.frame = frame;
  return field;
}

function textField(value, frame) {
  const field = $.NSTextField.alloc.initWithFrame(frame);
  field.stringValue = value;
  return field;
}

function writeValue(directory, name, value) {
  const path = directory + "/" + name;
  const text = $.NSString.stringWithString(String(value));
  if (!text.writeToFileAtomicallyEncodingError(path, true, $.NSUTF8StringEncoding, null)) {
    throw new Error("Could not save the selected " + name + ".");
  }
}

function run(argv) {
  const appName = argv[0];
  const defaultProject = argv[1];
  const outputDirectory = argv[2];

  $.NSApplication.sharedApplication;
  $.NSApp.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
  $.NSApp.activateIgnoringOtherApps(true);

  const alert = $.NSAlert.alloc.init;
  alert.messageText = "Create a " + appName + " project";
  alert.informativeText = "Review the settings, then choose Setup.";
  alert.addButtonWithTitle("Setup");
  alert.addButtonWithTitle("Cancel");

  const view = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, 620, 76));
  const agent = $.NSPopUpButton.alloc.initWithFramePullsDown($.NSMakeRect(145, 44, 465, 26), false);
  agent.addItemsWithTitles(["codex", "claude"]);
  const project = textField(defaultProject, $.NSMakeRect(145, 8, 465, 24));

  view.addSubview(label("Agent", $.NSMakeRect(0, 48, 135, 20)));
  view.addSubview(agent);
  view.addSubview(label("Project folder", $.NSMakeRect(0, 12, 135, 20)));
  view.addSubview(project);
  alert.accessoryView = view;

  if (Number(alert.runModal) !== Number($.NSAlertFirstButtonReturn)) {
    return "";
  }
  writeValue(outputDirectory, "agent", ObjC.unwrap(agent.titleOfSelectedItem));
  writeValue(outputDirectory, "project", ObjC.unwrap(project.stringValue));
  return "setup";
}
