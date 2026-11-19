# ---- Project library (run from the project root) ----
stopifnot(file.exists(".Renviron"), dir.exists("code"))
readRenviron(".Renviron")
dir.create(".r-library", showWarnings = FALSE)
Sys.setenv(R_LIBS_USER = normalizePath(".r-library"),
           RENV_PATHS_LIBRARY = normalizePath(".r-library"))
.libPaths(c(Sys.getenv("R_LIBS_USER"), .libPaths()))
if (file.exists("code/.sandbox/bioconductor-config.yaml")) {
  # Validate against the project's saved mapping without requiring an online probe.
  Sys.setenv(BIOCONDUCTOR_CONFIG_FILE = paste0("file://", normalizePath("code/.sandbox/bioconductor-config.yaml")))
}
if (file.exists("renv/activate.R")) source("renv/activate.R")
