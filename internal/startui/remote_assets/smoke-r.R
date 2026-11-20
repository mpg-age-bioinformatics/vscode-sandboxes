# Build-time regression for the Apple silicon / amd64 ICU mapping failure.
smoke <- function() {
  stopifnot(capabilities("ICU"))
  icuSetCollate(locale = "en_US")
  stopifnot(identical(sort(c("z", "\u00e9", "a")), c("a", "\u00e9", "z")))
  folder <- tempfile("vscode-r-smoke-")
  dir.create(folder)
  on.exit(unlink(folder, recursive = TRUE))
  x <- data.frame(value = 1:3)
  csv <- file.path(folder, "values.csv")
  write.csv(x, csv, row.names = FALSE)
  stopifnot(identical(read.csv(csv)$value, 1:3))
  for (format in c("pdf", "png")) {
    file <- file.path(folder, paste0("plot.", format))
    if (format == "pdf") pdf(file) else png(file, type = "cairo")
    plot(x$value)
    dev.off()
    stopifnot(file.info(file)$size > 0)
  }
  cat(R.version.string, "R_SMOKE_OK: ICU collation, CSV, PDF and PNG\n")
}
smoke()
