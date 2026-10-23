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
# ---- Restore an existing environment, or install initial editor tooling ----
if (file.exists("renv.lock")) {
  renv::restore(prompt = FALSE)
} else {
  # Install separately before any Bioconductor dependency resolution.
  renv::install("BiocManager@1.30.26", prompt = FALSE)
  renv::install(c("languageserver@0.3.18", "httpgd@2.1.4", "jsonlite@2.0.0"),
                prompt = FALSE)
  renv::snapshot(type = "all", prompt = FALSE)
}
# Verify resolution in a fresh process, including after restoring a lockfile.
status <- system2(file.path(R.home("bin"), "Rscript"), "code/verify-renv.R")
if (status != 0L) stop("Fresh-process project library verification failed")
