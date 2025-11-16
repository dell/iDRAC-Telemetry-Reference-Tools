import pytest
from commonhelper import local_exec

cleanup_container_list = ['influx', 'splunk', 'elk','grafana','timescale','victoria','prometheus']

class TestReferenceToolCleanup:

    """
        This function tests the cleanup of a container by stopping it if it's running, removing it, 
        and optionally pruning unused containers and volumes. It takes a container name as a 
        parameter and returns None.
    """
    @pytest.mark.parametrize("container", cleanup_container_list)
    def test_cleanup_selected_container(self, container):
        print(f"Cleaning up container: {container}")

        # Stop container if running
        local_exec(f"docker stop $(docker ps -q --filter name={container}) || true")

        # Remove container
        local_exec(f"docker rm $(docker ps -a -q --filter name={container}) || true")

        # Optional: prune unused containers and volumes
        local_exec("docker container prune -f")
        local_exec("docker volume prune -f")

        # Validate cleanup for this container
        out = local_exec("docker ps -a --format '{{.Names}}'")
        assert container not in out, f"Cleanup failed for {container}"
        print(f"{container} cleaned successfully")

    """
        This function tests the cleanup of a Docker network. It takes no parameters and returns None.
    """
    def test_cleanup_network(self):
        local_exec("docker network rm idrac-telemetry-reference-tools_host-bridge-net || true")
        networks = local_exec("docker network ls --format '{{.Name}}'")
        assert "idrac-telemetry-reference-tools_host-bridge-net" not in networks
        print("Network cleaned successfully")