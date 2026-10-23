# Run inside the sandbox: Rscript code/setup-renv.R
# ---- Activate project ----
source("code/activate-project.R")
options(repos = c(CRAN = "https://cloud.r-project.org"))
if (!file.exists("renv/activate.R")) {
  utils::install.packages(
    "https://cloud.r-project.org/src/contrib/Archive/renv/renv_1.1.5.tar.gz",
    repos = NULL, type = "source", lib = Sys.getenv("R_LIBS_USER"))
  renv::init(bare = TRUE, restart = FALSE)
}
renv::settings$snapshot.type("all")
# ---- Restore project dependencies ----
# Restore also compiles later on a fresh machine, so use the same scoped flags.
local({
  previous <- Sys.getenv("R_MAKEVARS_USER", unset = NA_character_)
  on.exit(if (is.na(previous)) Sys.unsetenv("R_MAKEVARS_USER") else
    Sys.setenv(R_MAKEVARS_USER = previous))
  flags <- tempfile("Makevars-later-")
  on.exit(unlink(flags), add = TRUE)
  if (!is.na(previous) && file.exists(previous)) {
    writeLines(readLines(previous, warn = FALSE), flags)
  } else {
    default <- path.expand("~/.R/Makevars")
    if (file.exists(default)) writeLines(readLines(default, warn = FALSE), flags)
  }
  # later's tinycthread conflicts with recent glibc declarations under C23.
  # Enable GNU APIs before the forced header, including ps CPU affinity macros.
  cat("\nCFLAGS += -std=gnu17 -D_GNU_SOURCE -include stdlib.h\n", file = flags, append = TRUE)
  Sys.setenv(R_MAKEVARS_USER = flags)
  if (file.exists("renv.lock")) {
    renv::restore(prompt = FALSE)
  } else {
    # Install separately before any Bioconductor dependency resolution.
    renv::install("BiocManager@1.30.26", prompt = FALSE)
    renv::install(c("languageserver@0.3.18", "httpgd@2.1.4", "jsonlite@2.0.0"),
                  prompt = FALSE)
  }
  # Required even when upgrading an older project's lockfile.
  renv::install(c("later@1.4.4", "rstudioapi@0.17.1", "processx@3.9.0"),
                prompt = FALSE)
})
renv::snapshot(type = "all", prompt = FALSE)
# Verify resolution in a fresh process, including after restoring a lockfile.
status <- system2(file.path(R.home("bin"), "Rscript"), "code/verify-renv.R")
if (status != 0L) stop("Fresh-process project library verification failed")
