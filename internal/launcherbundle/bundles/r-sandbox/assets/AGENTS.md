# R sandbox environment

- Put implementation files in `code/` and data or generated artifacts in `data/`.
- Use R as the default analysis language unless the task requires another tool.
- Treat Docker Sandbox setup as completed scaffolding. Do not create a Dev Container, rerun setup after ordinary changes, or use the host Docker socket.
- Run R inside Docker Sandbox. Use the project `.r-library/` through `R_LIBS_USER`; never install project packages on the host or globally in the sandbox.
- Record every package installation instruction in tracked code or a dependency manifest. Pin every package exactly; do not use ranges, unversioned packages, or `latest`.
- Use `renv`. Record exact installs such as `renv::install("dplyr@1.1.4")`, run `renv::snapshot()`, and track `renv.lock`. Update installation code and the lockfile together.
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
- Keep scripts reproducible from top to bottom from the project root. Use `View(object)` for tables and `httpgd` for interactive plots when appropriate.

- Use Podman for all workload container builds, runs, and reproduction inside the sandbox; do not use Docker CLI or a host container socket. Use `sudo env XDG_RUNTIME_DIR=/run/user/0 podman --storage-driver=vfs --events-backend=file` consistently so builds, runs, and exec share the same root-owned image/container store. For builds, add `build --isolation=chroot --network=host`. For runs, add `run --cgroups=disabled --network=host` (or `--network=none` when networking is unnecessary): the sandbox has a read-only cgroup filesystem and no delegated cgroups. Host networking here shares only the sandbox network, not the physical host network. Keep recipes and pinned dependency manifests under `code/`, use fully qualified registry image names, mount only required project paths, and record actual Podman build/run/exec commands for reproducibility. Dockerfiles remain valid Podman build recipes.
