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
