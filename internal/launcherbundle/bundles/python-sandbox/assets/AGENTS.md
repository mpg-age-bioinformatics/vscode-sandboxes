# Python sandbox environment

- Put implementation files, notebooks, tests, dependency manifests, and installation scripts under `code/`. Put input data and generated artifacts under `data/`.
- Use Python by default. Follow the notebook and Jupytext workflow below for Python analysis, visualization, exploration, and code-generation requests unless the user explicitly asks for another format.
- Treat the Docker Sandbox setup as completed scaffolding. Use the existing development container inside the general Docker Sandbox. Do not rerun scaffolding after ordinary changes or mount a host Docker socket.
- Run Python inside the Python development container using the project-root `.venv/`, stored in a sandbox-local Docker volume. Do not install project dependencies globally, with `sudo`, on the host, or into the sandbox system interpreter.
- Follow the dependency tracking workflow below for every package installation, upgrade, or removal, including temporary experiments and notebook tooling.
- Executable scripts must work from the project root in a fresh `.venv/bin/python` process without interactive shell state.
- For scientific, data-analysis, visualization, or bioinformatics work, use established libraries such as Pandas, NumPy, SciPy, Matplotlib, Seaborn, Biopython, and scikit-learn. Do not replace conventional library workflows with hand-written substitutes merely to avoid dependencies.
- Use pytest unless the project already uses another framework. Keep notebooks reproducible top to bottom, save artifacts under `data/`, and move substantial reusable logic into modules under `code/`.
- Keep secrets out of code, notebooks, output, Git, images, and templates. Treat the mounted repository as the only host path in scope.

- The outer general sandbox owns the Docker Engine. Build, pull, and run workload containers only through that sandbox-local engine, never a host engine or socket. VS Code opens the inner development container by default; its terminals and agent tools execute there. Keep configuration in `code/.sandbox/devcontainer.json`, record the resolved image in `code/.sandbox/container-image.lock.json`, and retain the generated Dev Containers feature lockfile under `code/.sandbox/`. Keep the Dockerfile actually used to build the development container at `code/Dockerfile`. The inner container has no Docker socket mounted. To manage additional sibling containers, use `sbx exec <sandbox-name> docker ...` from the host launcher/terminal; do not install host Docker or expose a daemon over TCP.

## Notebooks and synchronized Python files

- Start the requested work in a Jupyter notebook under `code/`. The notebook is the primary working document and deliverable; a standalone `.py` file alone does not satisfy the default workflow.
- Pair every working notebook with a same-directory, same-stem Jupytext Python file in percent format: for example, `code/analysis.ipynb` and `code/analysis.py`. Keep code cells as `# %%` and explanatory Markdown cells as `# %% [markdown]` in the Python mirror. Use Jupytext to create and synchronize the pair rather than manually maintaining two copies.
- Before creating or executing notebooks, add `jupytext` and `ipykernel` to the tracked dependencies, resolve and install their locked dependencies into `.venv/`, and verify that the editor uses this environment's kernel.
- Set up pairing with `.venv/bin/python -m jupytext --set-formats ipynb,py:percent code/analysis.ipynb`, substituting the actual notebook path. Synchronize with `.venv/bin/python -m jupytext --sync code/analysis.ipynb` after changes to either file and before execution or handoff.
- Edit one member of the pair at a time, then synchronize before editing the other. If both have unsynchronized edits, reconcile them before running synchronization; do not discard either file's work based only on modification times.
- Execute the notebook from a fresh kernel, top to bottom, using the project `.venv/`. Inspect errors and relevant outputs, then save the executed notebook and synchronize its Python mirror again. Keep both files in version control, with matching code and Markdown cells; notebook outputs remain in the `.ipynb` file. Report any execution that could not be completed.
- Keep substantial reusable logic in modules under `code/`, imported by the notebook. Store generated figures, tables, and other data artifacts under `data/`.

## Dependency and installation tracking

- Before installing anything, inspect the existing dependency manifests, lockfile, and installation records. Reuse the project's resolver and installation workflow. Do not treat an interactive terminal command, notebook `%pip`/`!pip` cell, or the current environment as the installation record.
- Maintain `code/requirements.in` for direct dependencies, with an exact version for each, and `code/requirements.lock` for the fully resolved dependencies, with every direct and transitive dependency pinned exactly. Include analysis packages, notebook/kernel tools, test tools, and the resolver tooling needed to reproduce the workflow. Update both files together whenever dependencies change; generate the lock with a resolver rather than hand-writing transitive dependencies or relying only on `pip freeze`.
- Resolve the updated manifest first, then install from `code/requirements.lock` into the project-root `.venv/` inside the development container. Never leave a successful ad hoc installation unrecorded. If a temporary package is no longer needed, remove it from the manifest, regenerate the lock, and reconcile the environment using the project's dependency tooling.
- Keep reproducible bootstrap, lock-generation, and installation commands in a script under `code/`. Record the exact resolver/tool versions and any required package indexes or platform constraints; use environment variables for credentials and never record their values.
- Maintain `code/INSTALL.md` as an installation history. For every installation, upgrade, or removal, record the date, package names and resolved versions (or a reference to the corresponding lockfile change), why the change was needed, the exact commands run, and whether installation and verification succeeded. Record failures and any unresolved setup steps too. This history supplements the manifests and lockfile.
- Track required OS packages and non-Python tools in `code/Dockerfile` or a reproducible installation script under `code/`, including versions where available, and document their installation in `code/INSTALL.md`. Apply container changes through the existing sandbox workflow.
- After changing dependencies, check the environment for dependency conflicts and verify the affected imports or notebook execution. Before handoff, ensure the manifest, lockfile, installation script, installation history, and notebook/Python pairs reflect the delivered work. Explicitly report unresolved dependency or reproducibility issues.

<!-- sandbox:repository-policy:start -->
- Never stage, force-add, or commit any directory whose name starts with `data`, at any depth, or any of its contents (including `data/` and `data_<hash>/`). Keep the `data*/` ignore rule and the data commit guard enabled. If data is already tracked, remove it from the Git index with `git rm --cached` while preserving the files on disk.
- Keep only `match-code-data` and `set-vscode-r-version` from the remote skills repository in `skills/`. Do not install or restore other remote skills.
<!-- sandbox:repository-policy:end -->
