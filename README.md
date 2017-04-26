Prometheus Process Exporter
===========================

The Prometheus Process Exporter picks up stats from all processes running
in the system (or container) and exports them as prometheus variables.

Currently, the following stats are exported:

 - node_os_process_memory: size of the process.
 - node_os_num_processes: number of running processes of the type.

The stats are updated once per minute. Each of the variables, unless
specified otherwise, is exported as a map with the process name as
the key. Multiple processes of the same type will be aggregated by
adding up their stats.
