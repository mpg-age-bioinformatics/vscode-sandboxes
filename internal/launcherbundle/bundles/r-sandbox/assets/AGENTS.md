# R sandbox environment

- Put implementation files in `code/` and data or generated artifacts in `data/`.
- Use R as the default analysis language unless the task requires another tool.
- Treat Docker Sandbox setup as completed scaffolding. Use the existing development container inside the general Docker Sandbox. Do not rerun scaffolding after ordinary changes or mount a host Docker socket.
- Run R inside the R development container, which is managed by Docker inside the general sandbox. Use the project `.r-library/` through `R_LIBS_USER`; it is backed by a sandbox-local Docker volume. Never install project packages on the host or into the outer sandbox.
- Record every package installation instruction in tracked code or a dependency manifest. Pin every package exactly; do not use ranges, unversioned packages, or `latest`.
- Use `renv`. Record exact installs such as `renv::install("dplyr@1.1.4")`, run `renv::snapshot(type = "all")`, and track `renv.lock`. Update installation code and the lockfile together.
- Before `library()` or `requireNamespace()`, affected scripts must activate the project-relative `renv/activate.R` or equivalently load the intended project library. Never hard-code an internal renv cache path.
- After dependency changes, run affected entry points in a fresh R process and verify new packages resolve from the intended project library with `find.package()` or `path.package()`.
<!-- r-sandbox:scientific-libraries:start -->
- Scientific analysis, bioinformatics, tabular-data handling, statistics, and visualization must use established CRAN or Bioconductor packages that express the work idiomatically. Installing and locking the appropriate packages is part of the task; package absence is never a reason to replace a conventional bioinformatics workflow with hand-written base-R plotting or data-processing code.
- For a CSV-backed heatmap, use `readr::read_csv()` (or an already established project tabular package) to read the data and use Bioconductor `ComplexHeatmap` with a direct `ComplexHeatmap::Heatmap(...)` call by default. Use `circlize::colorRamp2()` for an explicit centered color mapping when the values span zero. Include meaningful row and column labels, a named color legend, appropriate clustering or an explicit reason not to cluster, and save a conventional PNG or PDF under `data/` using `ComplexHeatmap::draw()`. `pheatmap::pheatmap()` is acceptable when the project already uses `pheatmap` or the user asks for it.
- Do not implement a requested heatmap with base `image()`, `heatmap()`, manually calculated cell coordinates, manual legend layout, or custom color interpolation merely to avoid dependencies. Do not manually parse CSV. If `readr`, `ComplexHeatmap`, `circlize`, or another required package is unavailable, add exact-version installation instructions under `code/`, install through the project `renv`, update `renv.lock`, and execute and verify the library-based workflow.
- In requests such as “create a CSV for a heatmap demo and then make me the heatmap,” deliver the CSV under `data/`, an editable sectioned R script under `code/`, the rendered heatmap under `data/`, and the tracked exact dependency setup. The script must read the saved CSV rather than plotting only an in-memory object.
<!-- r-sandbox:scientific-libraries:end -->
- Keep secrets out of source, output, Git, images, and templates. Treat the mounted repository as the only host path in scope.

## Interactive R development style

- Write analysis scripts as sectioned workflows that can be run incrementally, with user-editable configuration near the top and RStudio headers such as `# ---- Load data ----`.
- Keep intermediate objects inspectable, validate inputs after loading, and save outputs under `data/`.
- Do not use prompts, menus, `readline()`, or command-line arguments unless requested. “Interactive” means editable analysis code, not a prompt-driven program.
- Keep scripts reproducible from top to bottom from the project root. Use `View(object)` for tables. Follow the Dev Containers graphics workflow below for interactive plots.

- The outer general sandbox owns the Docker Engine. Build, pull, and run workload containers only through that sandbox-local engine, never a host engine or socket. VS Code opens the inner development container by default; its terminals and agent tools execute there. Keep configuration in `code/.sandbox/devcontainer.json`, record the resolved image in `code/.sandbox/container-image.lock.json`, and retain the generated Dev Containers feature lockfile under `code/.sandbox/`. Keep the Dockerfile actually used to build the development container at `code/Dockerfile`. The inner container has no Docker socket mounted. To manage additional sibling containers, use `sbx exec <sandbox-name> docker ...` from the host launcher/terminal; do not install host Docker or expose a daemon over TCP.

## Project dependencies and Dev Containers plots

- Run `Rscript code/.sandbox/setup-renv.R` inside the sandbox to install initial editor tooling or restore `renv.lock`; no sandbox rebuild is needed. Startup and scripts must use `source("code/.sandbox/activate-project.R")` before loading packages, from the project root. This activates `renv/activate.R` when initialized and uses `.r-library/` through both `R_LIBS_USER` and `RENV_PATHS_LIBRARY`.
- Keep the environment-specific working settings `RENV_CONFIG_INSTALL_TRANSACTIONAL=FALSE` and `RENV_CONFIG_CACHE_ENABLED=FALSE`. Do not generalize them to every R environment. Never hard-code renv cache paths.
- Install pinned BiocManager separately before resolving Bioconductor packages. Add exact scientific package versions appropriate to the selected R/Bioconductor release in `code/.sandbox/setup-renv.R` before snapshotting. Update that code and `renv.lock` together; include transitive dependencies and project-local `languageserver@0.3.18`, `httpgd@2.1.4`, and `jsonlite@2.0.0`, `later@1.4.4`, `rstudioapi@0.17.1`, and `processx@3.9.0`. Keep the scoped later compiler flags during both installation and restore; verify bundled `sess` loads under the activated project library before opening VS Code. Use snapshot type `"all"` consistently, including `renv::settings$snapshot.type("all")` for status checks. Run `Rscript code/.sandbox/verify-renv.R` and affected analyses in fresh processes after changes.
- Support Command+Enter in an attached R terminal in VS Code Dev Containers, both from the top and section by section. Keep an editable `display_plot <- interactive()` flag near the top. Draw and save independently: batch Rscript saves PNG/PDF without opening a viewer; interactive runs explicitly draw and leave the device open.
- With `r.plot.useHttpgd`, open the configured VS Code device as below. Do not call `httpgd::hgd()` directly for user-facing plots: it bypasses VS Code's viewer callback. Do not assume a remote localhost URL opens locally. Do not assert that the device name is `"httpgd"`; this version can report `"unigd"`.

```r
# ---- Configuration ----
display_plot <- interactive()
# ---- Activate packages ----
source("code/.sandbox/activate-project.R")
# ---- Display heatmap (after constructing heatmap) ----
if (display_plot) {
  if (!is.function(getOption("device"))) {
    stop("In VS Code Dev Containers run R: Create R terminal, attach it, then rerun this section.")
  }
  grDevices::dev.new(width = 10, height = 8.2, noRStudioGD = TRUE)
  ComplexHeatmap::draw(heatmap, merge_legends = TRUE,
                       padding = grid::unit(c(2, 12, 2, 2), "mm"))
}
```

- Use `merge_legends = TRUE` and sufficient right padding for both saved and interactive heatmaps. Close only file devices after saving; inspect rendered PNG/PDF visually for legend overlap, clipped titles, and readable labels.
- Keep `r.alwaysUseActiveTerminal` and `r.bracketedPaste` false for standard R. Bracketed paste is recommended for radian. The R banner and “Project loaded [renv …]” are normal output; do not attribute uncaptured disappearing notifications to them.
- Track setup code, `renv.lock`, `renv/activate.R`, `renv/settings.json`, configuration, and intended demo CSVs and rendered artifacts. Exclude installed libraries and caches. Do not rerun scaffolding for ordinary analysis or editor changes.

<!-- sandbox:repository-policy:start -->
- Never stage, force-add, or commit any directory whose name starts with `data`, at any depth, or any of its contents (including `data/` and `data_<hash>/`). Keep the `data*/` ignore rule and the data commit guard enabled. If data is already tracked, remove it from the Git index with `git rm --cached` while preserving the files on disk.
- Keep only `match-code-data` and `set-vscode-r-version` from the remote skills repository in `skills/`. Do not install or restore other remote skills.
<!-- sandbox:repository-policy:end -->
