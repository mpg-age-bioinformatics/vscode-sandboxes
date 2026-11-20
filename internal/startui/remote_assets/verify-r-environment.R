# Fast startup check; graphical device regression tests run separately.
source("code/.sandbox/activate-project.R")
expected <- c(languageserver = "0.3.18", httpgd = "2.1.4", jsonlite = "2.0.0",
              later = "1.4.4", rstudioapi = "0.17.1", processx = "3.9.0")
root <- paste0(normalizePath(".r-library"), "/")
for (package in names(expected)) {
  stopifnot(requireNamespace(package, quietly = TRUE),
            as.character(utils::packageVersion(package)) == expected[[package]],
            startsWith(normalizePath(find.package(package)), root))
}
stopifnot(identical(renv::settings$snapshot.type(), "all"))
cat("Remote R project dependencies verified.\n")
