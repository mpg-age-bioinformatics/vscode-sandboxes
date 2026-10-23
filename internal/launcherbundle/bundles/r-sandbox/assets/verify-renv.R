# Run inside the sandbox: Rscript code/verify-renv.R
source("code/activate-project.R")
expected <- c(languageserver = "0.3.18", httpgd = "2.1.4", jsonlite = "2.0.0",
              later = "1.4.4", rstudioapi = "0.17.1", processx = "3.9.0")
root <- paste0(normalizePath(".r-library"), "/")
for (package in names(expected)) {
  stopifnot(requireNamespace(package, quietly = TRUE),
            as.character(utils::packageVersion(package)) == expected[[package]],
            startsWith(normalizePath(find.package(package)), root))
}
# Smoke-test httpgd without assuming its device name or opening a remote URL.
httpgd::hgd(silent = TRUE)
tryCatch({
  graphics::plot(1:3, 1:3)
  stopifnot(grDevices::dev.cur() > 1L)
}, finally = grDevices::dev.off())
stopifnot(identical(renv::settings$snapshot.type(), "all"))
renv::status()
