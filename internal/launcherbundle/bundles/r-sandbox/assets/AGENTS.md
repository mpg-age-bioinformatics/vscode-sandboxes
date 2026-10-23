# R sandbox environment

- Put implementation files in `code/` and data or generated artifacts in `data/`.
- Use R as the default analysis language unless the task requires another tool.
- Treat Docker Sandbox setup as completed scaffolding. Do not create a Dev Container, rerun setup after ordinary changes, or use the host Docker socket.
- Run R inside Docker Sandbox. Use the project `.r-library/` through `R_LIBS_USER`; never install project packages on the host or globally in the sandbox.
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
- Keep scripts reproducible from top to bottom from the project root. Use `View(object)` for tables. Follow the Remote-SSH graphics workflow below for interactive plots.

- Use Podman for all workload container builds, runs, and reproduction inside the sandbox; do not use Docker CLI or a host container socket. Use `sudo env XDG_RUNTIME_DIR=/run/user/0 podman --storage-driver=vfs --events-backend=file` consistently so builds, runs, and exec share the same root-owned image/container store. For builds, add `build --isolation=chroot --network=host`. For runs, add `run --cgroups=disabled --network=host` (or `--network=none` when networking is unnecessary): the sandbox has a read-only cgroup filesystem and no delegated cgroups. Host networking here shares only the sandbox network, not the physical host network. Keep recipes and pinned dependency manifests under `code/`, use fully qualified registry image names, mount only required project paths, and record actual Podman build/run/exec commands for reproducibility. Dockerfiles remain valid Podman build recipes.

## Project dependencies and Remote-SSH plots

- Run `Rscript code/setup-renv.R` inside the sandbox to install initial editor tooling or restore `renv.lock`; no sandbox rebuild is needed. Startup and scripts must use `source("code/activate-project.R")` before loading packages, from the project root. This activates `renv/activate.R` when initialized and uses `.r-library/` through both `R_LIBS_USER` and `RENV_PATHS_LIBRARY`.
- Keep the environment-specific working settings `RENV_CONFIG_INSTALL_TRANSACTIONAL=FALSE` and `RENV_CONFIG_CACHE_ENABLED=FALSE`. Do not generalize them to every R environment. Never hard-code renv cache paths.
- Install pinned BiocManager separately before resolving Bioconductor packages. Add exact scientific package versions appropriate to the selected R/Bioconductor release in `code/setup-renv.R` before snapshotting. Update that code and `renv.lock` together; include transitive dependencies and project-local `languageserver@0.3.18`, `httpgd@2.1.4`, and `jsonlite@2.0.0`. Use snapshot type `"all"` consistently, including `renv::settings$snapshot.type("all")` for status checks. Run `Rscript code/verify-renv.R` and affected analyses in fresh processes after changes.
- Support Command+Enter in an attached R terminal in VS Code Remote-SSH, both from the top and section by section. Keep an editable `display_plot <- interactive()` flag near the top. Draw and save independently: batch Rscript saves PNG/PDF without opening a viewer; interactive runs explicitly draw and leave the device open.
- With `r.plot.useHttpgd`, open the configured VS Code device as below. Do not call `httpgd::hgd()` directly for user-facing plots: it bypasses VS Code's viewer callback. Do not assume a remote localhost URL opens locally. Do not assert that the device name is `"httpgd"`; this version can report `"unigd"`.

```r
# ---- Configuration ----
display_plot <- interactive()
# ---- Activate packages ----
source("code/activate-project.R")
# ---- Display heatmap (after constructing heatmap) ----
if (display_plot) {
  if (!is.function(getOption("device"))) {
    stop("In VS Code Remote-SSH run R: Create R terminal, attach it, then rerun this section.")
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
