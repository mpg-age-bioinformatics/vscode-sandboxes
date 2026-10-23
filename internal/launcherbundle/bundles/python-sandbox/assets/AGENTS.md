# Python sandbox environment

- Put implementation files, notebooks, tests, dependency manifests, and installation scripts under `code/`. Put input data and generated artifacts under `data/`.
- Use Python by default. Use `code/*.ipynb` as the default development environment and primary deliverable for Python analysis, visualization, exploration, and code-generation requests unless the user asks for another format.
- Treat the Docker Sandbox setup as completed scaffolding. Do not create a Dev Container, rerun setup after ordinary changes, or use the host Docker socket.
- Run Python inside the sandbox using the project-root `.venv/`. Do not install project dependencies globally, with `sudo`, on the host, or into the sandbox system interpreter.
- Track every dependency and installation instruction under `code/`. Pin every direct and transitive dependency exactly. Maintain `code/requirements.in` and a fully resolved `code/requirements.lock`; install from the lock and update both together.
- Executable scripts must work from the project root in a fresh `.venv/bin/python` process without interactive shell state.
- Before creating or executing a notebook, track `ipykernel`, resolve and install its locked dependencies into `.venv/`, and verify the editor can use that kernel.
- For scientific, data-analysis, visualization, or bioinformatics work, use established libraries such as Pandas, NumPy, SciPy, Matplotlib, Seaborn, Biopython, and scikit-learn. Do not replace conventional library workflows with hand-written substitutes merely to avoid dependencies.
- Use pytest unless the project already uses another framework. Keep notebooks reproducible top to bottom, save artifacts under `data/`, and move substantial reusable logic into modules under `code/`.
- Keep secrets out of code, notebooks, output, Git, images, and templates. Treat the mounted repository as the only host path in scope.

- Use Podman for all workload container builds, runs, and reproduction inside the sandbox; do not use Docker CLI or a host container socket. Use `sudo env XDG_RUNTIME_DIR=/run/user/0 podman --storage-driver=vfs --events-backend=file` consistently so builds, runs, and exec share the same root-owned image/container store. For builds, add `build --isolation=chroot --network=host`. For runs, add `run --cgroups=disabled --network=host` (or `--network=none` when networking is unnecessary): the sandbox has a read-only cgroup filesystem and no delegated cgroups. Host networking here shares only the sandbox network, not the physical host network. Keep recipes and pinned dependency manifests under `code/`, use fully qualified registry image names, mount only required project paths, and record actual Podman build/run/exec commands for reproducibility. Dockerfiles remain valid Podman build recipes.

<!-- sandbox:repository-policy:start -->
- Never stage, force-add, or commit any directory whose name starts with `data`, at any depth, or any of its contents (including `data/` and `data_<hash>/`). Keep the `data*/` ignore rule and the data commit guard enabled. If data is already tracked, remove it from the Git index with `git rm --cached` while preserving the files on disk.
- Keep only `match-code-data` and `set-vscode-r-version` from the remote skills repository in `skills/`. Do not install or restore other remote skills.
<!-- sandbox:repository-policy:end -->
