# Rscript --vanilla scripts/test-r-session-setup.R [setup-vscode-r.R]
# Regression: an existing .ready marker must not bypass namespace loading.
args <- commandArgs(TRUE)
setup <- normalizePath(if (length(args)) args[[1L]] else
  "internal/launcherbundle/bundles/r-sandbox/assets/setup-vscode-r.R", mustWork = TRUE)
local({
  project <- tempfile("session-setup-test-")
  dir.create(project)
  previous <- getwd()
  old <- Sys.getenv("VSCODE_AGENT_FOLDER", unset = NA_character_)
  on.exit({
    setwd(previous)
    if (is.na(old)) Sys.unsetenv("VSCODE_AGENT_FOLDER") else
      Sys.setenv(VSCODE_AGENT_FOLDER = old)
    unlink(project, recursive = TRUE)
  })
  setwd(project)
  dir.create("code")
  writeLines("# Fixture activation", "code/activate-project.R")
  root <- file.path(project, "server")
  extension <- file.path(root, "extensions", "reditorsupport.r-999.0.0")
  package <- file.path(extension, "dist", "resources", "sess")
  dir.create(package, recursive = TRUE)
  dir.create(file.path(extension, "R"))
  writeLines(c("Package: sess", "Version: 1.0.0"), file.path(package, "DESCRIPTION"))
  writeLines(c(
    'sess_source_revision <- function(...) "fixture"',
    'sess_managed_library <- function(root, revision) file.path(root, revision, "library")',
    'sess_find_source_library <- function(...) "fixture"',
    'sess_load_namespace <- function(...) stop("fixture missing dependency")'
  ), file.path(extension, "R", "sess_source.R"))
  writeLines('sess_verify_package <- function(...) invisible(TRUE)',
             file.path(extension, "R", "sess-package-install.R"))
  parent <- file.path(root, "data", "User", "globalStorage", "reditorsupport.r", "sess", "fixture")
  dir.create(parent, recursive = TRUE)
  Sys.setenv(VSCODE_AGENT_FOLDER = root)
  for (ready in c(FALSE, TRUE)) {
    marker <- file.path(parent, ".ready")
    if (ready) writeLines("fixture", marker) else unlink(marker)
    error <- tryCatch({ source(setup); NULL }, error = identity)
    stopifnot(inherits(error, "error"),
              grepl("fixture missing dependency", conditionMessage(error)),
              identical(file.exists(marker), ready),
              !dir.exists(file.path(parent, ".setup-lock")))
  }
  cat("Session load regression checks passed (new setup and existing marker).\n")
})
