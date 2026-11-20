#!/bin/bash
# Stub for the Slurm client commands inside the AI agents container.
#
# The container is started with '--containall' and does not bind-mount the
# host's Slurm client tools, configuration or daemon sockets, so Slurm cannot
# work from inside it. This script is hardlinked to all Slurm command names
# (see Apptainer.agents.def) to replace the bare 'command not found' with an
# explanation of what to do instead - mainly for the benefit of AI agents,
# which otherwise start looking for workarounds.

set -e

CMD="$(basename "$0")"
if [[ -z "$CMD" || "$CMD" == "slurm-unavailable" ]]
then
    SUBJECT="This Slurm command"
else
    SUBJECT="'${CMD}'"
fi

cat >&2 <<EOF

=====================================================================
  Slurm is NOT available inside the AI agents container
=====================================================================

${SUBJECT} cannot be used from within this container: the host's
Slurm client tools, configuration and daemon sockets are deliberately
not mounted into it. This is by design and cannot be worked around
from inside the container.

For AI agents:
  Do not try to submit, cancel or query jobs yourself, and do not look
  for another way in. Write the job script (e.g. 'job.slurm') into the
  working directory and ask the user to submit it from a shell outside
  the container. Once the job has run, you can read its output files
  from inside the container as usual and continue the analysis there.

For users:
  Submit and monitor your jobs from a regular shell on the login node,
  outside the container:

      sbatch job.slurm
      squeue --me

Short test runs can be done interactively inside the container instead:
single-node MPI support is always enabled, and GPU support is switched on
automatically on a node that has GPUs.

See the 'Slurm batch system' section of the container README.

EOF

exit 1
