# Run inside the sandbox with Rscript --vanilla code/setup-vscode-r.R.
# Editor infrastructure belongs in vscode-R's private library, outside renv.
local({
  roots <- unique(c(Sys.getenv("VSCODE_AGENT_FOLDER", unset = ""),
                    path.expand("~/.vscode-server"),
                    path.expand("~/.vscode-server-insiders")))
  roots <- roots[nzchar(roots) & dir.exists(roots)]
  for (root in roots) {
    extensions <- list.dirs(file.path(root, "extensions"),
                            full.names = TRUE, recursive = FALSE)
    extensions <- extensions[grepl("^reditorsupport\\.r-[0-9]", basename(extensions))]
    for (extension in extensions) {
      package <- file.path(extension, "dist", "resources", "sess")
      helper <- file.path(extension, "R", "sess_source.R")
      installer <- file.path(extension, "R", "sess-package-install.R")
      # Older vscode-R releases do not use bundled sess.
      if (!file.exists(file.path(package, "DESCRIPTION"))) next
      if (!all(file.exists(c(helper, installer)))) {
        stop("Incomplete vscode-R installation: ", extension)
      }
      api <- new.env(parent = globalenv())
      sys.source(helper, api)
      sys.source(installer, api)
      revision <- api$sess_source_revision(file.path(package, "DESCRIPTION"))
      if (is.null(revision)) stop("Invalid bundled sess revision: ", extension)
      managed_root <- file.path(root, "data", "User", "globalStorage",
                                "reditorsupport.r", "sess")
      library <- api$sess_managed_library(managed_root, revision)
      parent <- dirname(library)
      ready <- file.path(parent, ".ready")
      complete <- function() {
        file.exists(ready) &&
          identical(readLines(ready, warn = FALSE), revision) &&
          !is.null(api$sess_find_source_library(revision, library))
      }
      if (complete()) next
      dir.create(parent, recursive = TRUE, showWarnings = FALSE)
      lock <- file.path(parent, ".setup-lock")
      deadline <- Sys.time() + 300
      while (!dir.create(lock, showWarnings = FALSE, mode = "0700")) {
        if (Sys.time() >= deadline) stop("Timed out waiting for sess setup: ", lock)
        Sys.sleep(0.2)
      }
      tryCatch({
        if (!complete()) {
          dir.create(library, recursive = TRUE, showWarnings = FALSE)
          description <- read.dcf(file.path(package, "DESCRIPTION"))
          if (is.null(api$sess_find_source_library(revision, library))) {
            api$sess_install(package, library, "https://cloud.r-project.org")
          } else {
            api$sess_verify_package(library, description[1L, "Version"], revision, FALSE)
          }
          temporary <- tempfile(".ready-", tmpdir = parent)
          writeLines(revision, temporary, useBytes = TRUE)
          Sys.chmod(temporary, "0600")
          if (!file.rename(temporary, ready)) stop("Cannot publish sess setup marker")
          message("Prepared vscode-R session watcher: ", revision)
        }
      }, finally = unlink(lock, recursive = TRUE))
    }
  }
})
