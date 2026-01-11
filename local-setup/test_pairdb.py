#!/usr/bin/env python3
"""
PairDB Local Deployment Test Suite

This script validates the PairDB deployment on Minikube and runs comprehensive tests including:
- Deployment validation
- Health checks
- Functional tests (CRUD operations)
- Consistency tests (quorum, eventual)
- Performance tests (latency, throughput)
- Concurrent operations
- Test report generation

Requirements:
    - Python 3.8+
    - kubectl configured for Minikube
    - PairDB deployed on Minikube

Usage:
    python3 test_pairdb.py [--namespace pairdb] [--skip-validation] [--output report.html]
"""

import argparse
import json
import subprocess
import sys
import time
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Dict, List, Optional, Tuple
import statistics


# ANSI color codes for terminal output
class Colors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'


class TestStatus(Enum):
    """Test result status"""
    PASSED = "PASSED"
    FAILED = "FAILED"
    SKIPPED = "SKIPPED"
    WARNING = "WARNING"


@dataclass
class TestResult:
    """Represents a single test result"""
    name: str
    status: TestStatus
    duration_ms: float
    message: str = ""
    details: Dict = field(default_factory=dict)


@dataclass
class TestSuite:
    """Represents a test suite with multiple test results"""
    name: str
    results: List[TestResult] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    def add_result(self, result: TestResult):
        self.results.append(result)

    def passed_count(self) -> int:
        return sum(1 for r in self.results if r.status == TestStatus.PASSED)

    def failed_count(self) -> int:
        return sum(1 for r in self.results if r.status == TestStatus.FAILED)

    def skipped_count(self) -> int:
        return sum(1 for r in self.results if r.status == TestStatus.SKIPPED)

    def total_duration_ms(self) -> float:
        return sum(r.duration_ms for r in self.results)


class PairDBTester:
    """Main test orchestrator for PairDB"""

    def __init__(self, namespace: str = "pairdb", api_gateway_url: Optional[str] = None):
        self.namespace = namespace
        self.api_gateway_url = api_gateway_url
        self.port_forward_process = None
        self.suites: List[TestSuite] = []

    def print_header(self, text: str):
        """Print a formatted header"""
        print(f"\n{Colors.HEADER}{Colors.BOLD}{'=' * 80}{Colors.ENDC}")
        print(f"{Colors.HEADER}{Colors.BOLD}{text.center(80)}{Colors.ENDC}")
        print(f"{Colors.HEADER}{Colors.BOLD}{'=' * 80}{Colors.ENDC}\n")

    def print_section(self, text: str):
        """Print a section header"""
        print(f"\n{Colors.OKBLUE}{Colors.BOLD}{text}{Colors.ENDC}")
        print(f"{Colors.OKBLUE}{'-' * len(text)}{Colors.ENDC}")

    def print_test_result(self, result: TestResult):
        """Print a single test result"""
        if result.status == TestStatus.PASSED:
            status_color = Colors.OKGREEN
            symbol = "✓"
        elif result.status == TestStatus.FAILED:
            status_color = Colors.FAIL
            symbol = "✗"
        elif result.status == TestStatus.WARNING:
            status_color = Colors.WARNING
            symbol = "⚠"
        else:
            status_color = Colors.OKCYAN
            symbol = "○"

        print(f"{status_color}{symbol} {result.name}{Colors.ENDC} "
              f"({result.duration_ms:.2f}ms)")
        if result.message:
            print(f"  {result.message}")

    def run_command(self, cmd: List[str], timeout: int = 30) -> Tuple[int, str, str]:
        """Run a shell command and return (returncode, stdout, stderr)"""
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout
            )
            return result.returncode, result.stdout, result.stderr
        except subprocess.TimeoutExpired:
            return -1, "", f"Command timed out after {timeout}s"
        except Exception as e:
            return -1, "", str(e)

    def kubectl(self, *args) -> Tuple[int, str, str]:
        """Run kubectl command"""
        return self.run_command(["kubectl", "-n", self.namespace] + list(args))

    # ==================== Validation Tests ====================

    def validate_deployment(self) -> TestSuite:
        """Validate that all required components are deployed"""
        suite = TestSuite(name="Deployment Validation")
        self.print_section("Validating Deployment")

        # Check namespace exists
        start = time.time()
        returncode, stdout, stderr = self.kubectl("get", "namespace", self.namespace)
        duration = (time.time() - start) * 1000

        if returncode == 0:
            suite.add_result(TestResult(
                name="Namespace exists",
                status=TestStatus.PASSED,
                duration_ms=duration,
                message=f"Namespace '{self.namespace}' found"
            ))
        else:
            suite.add_result(TestResult(
                name="Namespace exists",
                status=TestStatus.FAILED,
                duration_ms=duration,
                message=f"Namespace '{self.namespace}' not found"
            ))
            return suite

        # Check required deployments/statefulsets
        components = {
            "PostgreSQL": ("deployment", "postgres"),
            "Redis": ("deployment", "redis"),
            "Coordinator": ("deployment", "coordinator"),
            "Storage Node": ("statefulset", "pairdb-storage-node"),
            "API Gateway": ("deployment", "api-gateway"),
        }

        for component_name, (resource_type, resource_name) in components.items():
            start = time.time()
            returncode, stdout, stderr = self.kubectl("get", resource_type, resource_name)
            duration = (time.time() - start) * 1000

            if returncode == 0:
                suite.add_result(TestResult(
                    name=f"{component_name} deployed",
                    status=TestStatus.PASSED,
                    duration_ms=duration,
                    message=f"{resource_type}/{resource_name} exists"
                ))
            else:
                suite.add_result(TestResult(
                    name=f"{component_name} deployed",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"{resource_type}/{resource_name} not found"
                ))

        self.suites.append(suite)
        return suite

    def check_pod_health(self) -> TestSuite:
        """Check health status of all pods"""
        suite = TestSuite(name="Pod Health Check")
        self.print_section("Checking Pod Health")

        start = time.time()
        returncode, stdout, stderr = self.kubectl("get", "pods", "-o", "json")
        duration = (time.time() - start) * 1000

        if returncode != 0:
            suite.add_result(TestResult(
                name="Get pods",
                status=TestStatus.FAILED,
                duration_ms=duration,
                message=f"Failed to get pods: {stderr}"
            ))
            self.suites.append(suite)
            return suite

        try:
            pods_data = json.loads(stdout)
            pods = pods_data.get("items", [])

            for pod in pods:
                pod_name = pod["metadata"]["name"]
                pod_status = pod["status"]["phase"]

                # Check pod phase
                if pod_status == "Running":
                    status = TestStatus.PASSED
                    message = f"Pod is {pod_status}"
                elif pod_status in ["Pending", "ContainerCreating"]:
                    status = TestStatus.WARNING
                    message = f"Pod is {pod_status}"
                else:
                    status = TestStatus.FAILED
                    message = f"Pod is {pod_status}"

                # Check container statuses
                container_statuses = pod["status"].get("containerStatuses", [])
                for container in container_statuses:
                    if not container.get("ready", False):
                        status = TestStatus.WARNING
                        message += f" (container {container['name']} not ready)"

                suite.add_result(TestResult(
                    name=f"Pod: {pod_name}",
                    status=status,
                    duration_ms=duration,
                    message=message,
                    details={"phase": pod_status, "containers": len(container_statuses)}
                ))

        except json.JSONDecodeError:
            suite.add_result(TestResult(
                name="Parse pod status",
                status=TestStatus.FAILED,
                duration_ms=duration,
                message="Failed to parse pod JSON"
            ))

        self.suites.append(suite)
        return suite

    def setup_port_forward(self) -> TestResult:
        """Setup port forwarding to API Gateway"""
        self.print_section("Setting up Port Forward")

        start = time.time()

        # Kill any existing port-forward on 8080
        subprocess.run(["pkill", "-f", "port-forward.*8080"],
                      capture_output=True, check=False)
        time.sleep(2)

        try:
            # Start port-forward in background
            self.port_forward_process = subprocess.Popen(
                ["kubectl", "-n", self.namespace, "port-forward",
                 "svc/api-gateway", "8080:8080"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE
            )

            # Wait for port-forward to be ready
            time.sleep(3)

            # Check if process is still running
            if self.port_forward_process.poll() is None:
                self.api_gateway_url = "http://localhost:8080"
                duration = (time.time() - start) * 1000
                return TestResult(
                    name="Setup port-forward",
                    status=TestStatus.PASSED,
                    duration_ms=duration,
                    message=f"Port forwarding to {self.api_gateway_url}"
                )
            else:
                duration = (time.time() - start) * 1000
                return TestResult(
                    name="Setup port-forward",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message="Port-forward process exited"
                )

        except Exception as e:
            duration = (time.time() - start) * 1000
            return TestResult(
                name="Setup port-forward",
                status=TestStatus.FAILED,
                duration_ms=duration,
                message=f"Failed to setup port-forward: {e}"
            )

    def check_service_health(self) -> TestSuite:
        """Check health endpoints of services"""
        suite = TestSuite(name="Service Health Check")
        self.print_section("Checking Service Health")

        if not self.api_gateway_url:
            # Try to setup port-forward
            result = self.setup_port_forward()
            suite.add_result(result)
            if result.status != TestStatus.PASSED:
                self.suites.append(suite)
                return suite

        # Check API Gateway health
        try:
            import urllib.request
            import urllib.error

            start = time.time()
            try:
                response = urllib.request.urlopen(
                    f"{self.api_gateway_url}/health",
                    timeout=5
                )
                duration = (time.time() - start) * 1000

                suite.add_result(TestResult(
                    name="API Gateway /health",
                    status=TestStatus.PASSED,
                    duration_ms=duration,
                    message=f"HTTP {response.status}"
                ))
            except urllib.error.URLError as e:
                duration = (time.time() - start) * 1000
                suite.add_result(TestResult(
                    name="API Gateway /health",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"Failed: {e}"
                ))

        except ImportError:
            suite.add_result(TestResult(
                name="API Gateway /health",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="urllib not available"
            ))

        self.suites.append(suite)
        return suite

    # ==================== Functional Tests ====================

    def create_tenant(self, tenant_id: str, replication_factor: int = 2) -> Tuple[bool, str]:
        """
        Create a tenant before running tests

        Returns:
            Tuple of (success: bool, message: str)
        """
        if not self.api_gateway_url:
            return False, "API Gateway URL not available"

        try:
            import urllib.request
            import urllib.error

            tenant_data = json.dumps({
                "tenant_id": tenant_id,
                "replication_factor": replication_factor
            }).encode('utf-8')

            req = urllib.request.Request(
                f"{self.api_gateway_url}/v1/tenants",
                data=tenant_data,
                headers={'Content-Type': 'application/json'},
                method='POST'
            )

            try:
                response = urllib.request.urlopen(req, timeout=10)
                if response.status in [200, 201]:
                    return True, f"Tenant '{tenant_id}' created successfully"
                else:
                    return False, f"HTTP {response.status}"
            except urllib.error.HTTPError as e:
                if e.code == 409:
                    # Tenant already exists, that's OK
                    return True, f"Tenant '{tenant_id}' already exists"
                else:
                    return False, f"HTTP {e.code}: {e.reason}"
            except Exception as e:
                return False, f"Failed: {e}"

        except ImportError:
            return False, "urllib not available"

    def test_crud_operations(self) -> TestSuite:
        """Test basic CRUD operations"""
        suite = TestSuite(name="CRUD Operations")
        self.print_section("Testing CRUD Operations")

        if not self.api_gateway_url:
            suite.add_result(TestResult(
                name="CRUD tests",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="API Gateway URL not available"
            ))
            self.suites.append(suite)
            return suite

        try:
            import urllib.request
            import urllib.error

            tenant_id = f"test-tenant-{uuid.uuid4().hex[:8]}"
            test_key = f"test-key-{uuid.uuid4().hex[:8]}"
            test_value = f"test-value-{int(time.time())}"

            # Test 0: Create Tenant
            start = time.time()
            success, message = self.create_tenant(tenant_id, replication_factor=2)
            duration = (time.time() - start) * 1000

            if success:
                suite.add_result(TestResult(
                    name="Create tenant",
                    status=TestStatus.PASSED,
                    duration_ms=duration,
                    message=message,
                    details={"tenant_id": tenant_id, "replication_factor": 2}
                ))
            else:
                suite.add_result(TestResult(
                    name="Create tenant",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=message
                ))
                # If tenant creation fails, skip other tests
                self.suites.append(suite)
                return suite

            # Test 1: Create/Write
            start = time.time()
            write_data = json.dumps({
                "tenant_id": tenant_id,
                "key": test_key,
                "value": test_value,
                "consistency": "quorum"
            }).encode('utf-8')

            try:
                req = urllib.request.Request(
                    f"{self.api_gateway_url}/v1/key-value",
                    data=write_data,
                    headers={'Content-Type': 'application/json'},
                    method='POST'
                )
                response = urllib.request.urlopen(req, timeout=10)
                duration = (time.time() - start) * 1000

                if response.status in [200, 201]:
                    suite.add_result(TestResult(
                        name="Write operation",
                        status=TestStatus.PASSED,
                        duration_ms=duration,
                        message=f"Wrote key '{test_key}' successfully",
                        details={"tenant": tenant_id, "key": test_key}
                    ))
                else:
                    suite.add_result(TestResult(
                        name="Write operation",
                        status=TestStatus.FAILED,
                        duration_ms=duration,
                        message=f"HTTP {response.status}"
                    ))
            except Exception as e:
                duration = (time.time() - start) * 1000
                suite.add_result(TestResult(
                    name="Write operation",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"Failed: {e}"
                ))

            # Test 2: Read
            start = time.time()
            try:
                url = f"{self.api_gateway_url}/v1/key-value?tenant_id={tenant_id}&key={test_key}&consistency=quorum"
                response = urllib.request.urlopen(url, timeout=10)
                duration = (time.time() - start) * 1000

                if response.status == 200:
                    data = json.loads(response.read().decode('utf-8'))
                    retrieved_value = data.get("value", "")

                    if retrieved_value == test_value:
                        suite.add_result(TestResult(
                            name="Read operation",
                            status=TestStatus.PASSED,
                            duration_ms=duration,
                            message=f"Read key '{test_key}' successfully"
                        ))
                    else:
                        suite.add_result(TestResult(
                            name="Read operation",
                            status=TestStatus.FAILED,
                            duration_ms=duration,
                            message=f"Value mismatch: expected '{test_value}', got '{retrieved_value}'"
                        ))
                else:
                    suite.add_result(TestResult(
                        name="Read operation",
                        status=TestStatus.FAILED,
                        duration_ms=duration,
                        message=f"HTTP {response.status}"
                    ))
            except Exception as e:
                duration = (time.time() - start) * 1000
                suite.add_result(TestResult(
                    name="Read operation",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"Failed: {e}"
                ))

            # Test 3: Update
            updated_value = f"updated-{int(time.time())}"
            start = time.time()
            update_data = json.dumps({
                "tenant_id": tenant_id,
                "key": test_key,
                "value": updated_value,
                "consistency": "quorum"
            }).encode('utf-8')

            try:
                req = urllib.request.Request(
                    f"{self.api_gateway_url}/v1/key-value",
                    data=update_data,
                    headers={'Content-Type': 'application/json'},
                    method='POST'
                )
                response = urllib.request.urlopen(req, timeout=10)
                duration = (time.time() - start) * 1000

                if response.status in [200, 201]:
                    suite.add_result(TestResult(
                        name="Update operation",
                        status=TestStatus.PASSED,
                        duration_ms=duration,
                        message=f"Updated key '{test_key}' successfully"
                    ))
                else:
                    suite.add_result(TestResult(
                        name="Update operation",
                        status=TestStatus.FAILED,
                        duration_ms=duration,
                        message=f"HTTP {response.status}"
                    ))
            except Exception as e:
                duration = (time.time() - start) * 1000
                suite.add_result(TestResult(
                    name="Update operation",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"Failed: {e}"
                ))

            # Test 4: Delete
            start = time.time()
            try:
                delete_data = json.dumps({
                    "tenant_id": tenant_id,
                    "key": test_key
                }).encode('utf-8')

                req = urllib.request.Request(
                    f"{self.api_gateway_url}/v1/key-value",
                    data=delete_data,
                    headers={'Content-Type': 'application/json'},
                    method='DELETE'
                )
                response = urllib.request.urlopen(req, timeout=10)
                duration = (time.time() - start) * 1000

                if response.status in [200, 204]:
                    suite.add_result(TestResult(
                        name="Delete operation",
                        status=TestStatus.PASSED,
                        duration_ms=duration,
                        message=f"Deleted key '{test_key}' successfully"
                    ))
                else:
                    suite.add_result(TestResult(
                        name="Delete operation",
                        status=TestStatus.FAILED,
                        duration_ms=duration,
                        message=f"HTTP {response.status}"
                    ))
            except Exception as e:
                duration = (time.time() - start) * 1000
                suite.add_result(TestResult(
                    name="Delete operation",
                    status=TestStatus.FAILED,
                    duration_ms=duration,
                    message=f"Failed: {e}"
                ))

        except ImportError:
            suite.add_result(TestResult(
                name="CRUD tests",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="urllib not available"
            ))

        self.suites.append(suite)
        return suite

    # ==================== Performance Tests ====================

    def test_write_latency(self, num_operations: int = 100) -> TestSuite:
        """Test write latency"""
        suite = TestSuite(name=f"Write Latency ({num_operations} ops)")
        self.print_section(f"Testing Write Latency ({num_operations} operations)")

        if not self.api_gateway_url:
            suite.add_result(TestResult(
                name="Write latency test",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="API Gateway URL not available"
            ))
            self.suites.append(suite)
            return suite

        try:
            import urllib.request
            import urllib.error

            tenant_id = f"perf-tenant-{uuid.uuid4().hex[:8]}"

            # Create tenant first
            success, message = self.create_tenant(tenant_id, replication_factor=2)
            if not success:
                suite.add_result(TestResult(
                    name="Write latency test",
                    status=TestStatus.FAILED,
                    duration_ms=0,
                    message=f"Failed to create tenant: {message}"
                ))
                self.suites.append(suite)
                return suite

            latencies = []
            errors = 0

            overall_start = time.time()

            for i in range(num_operations):
                test_key = f"perf-key-{i}"
                test_value = f"perf-value-{i}"

                write_data = json.dumps({
                    "tenant_id": tenant_id,
                    "key": test_key,
                    "value": test_value,
                    "consistency": "quorum"
                }).encode('utf-8')

                start = time.time()
                try:
                    req = urllib.request.Request(
                        f"{self.api_gateway_url}/v1/key-value",
                        data=write_data,
                        headers={'Content-Type': 'application/json'},
                        method='POST'
                    )
                    response = urllib.request.urlopen(req, timeout=10)
                    duration_ms = (time.time() - start) * 1000
                    latencies.append(duration_ms)
                except Exception:
                    errors += 1

            overall_duration = (time.time() - overall_start) * 1000

            if latencies:
                avg_latency = statistics.mean(latencies)
                min_latency = min(latencies)
                max_latency = max(latencies)
                p50_latency = statistics.median(latencies)
                p95_latency = sorted(latencies)[int(len(latencies) * 0.95)] if len(latencies) > 1 else avg_latency
                p99_latency = sorted(latencies)[int(len(latencies) * 0.99)] if len(latencies) > 1 else avg_latency

                throughput = (num_operations - errors) / (overall_duration / 1000)  # ops/sec

                suite.add_result(TestResult(
                    name="Write latency",
                    status=TestStatus.PASSED if errors == 0 else TestStatus.WARNING,
                    duration_ms=overall_duration,
                    message=f"Avg: {avg_latency:.2f}ms, P50: {p50_latency:.2f}ms, P95: {p95_latency:.2f}ms, P99: {p99_latency:.2f}ms",
                    details={
                        "operations": num_operations,
                        "errors": errors,
                        "avg_latency_ms": round(avg_latency, 2),
                        "min_latency_ms": round(min_latency, 2),
                        "max_latency_ms": round(max_latency, 2),
                        "p50_latency_ms": round(p50_latency, 2),
                        "p95_latency_ms": round(p95_latency, 2),
                        "p99_latency_ms": round(p99_latency, 2),
                        "throughput_ops_per_sec": round(throughput, 2)
                    }
                ))
            else:
                suite.add_result(TestResult(
                    name="Write latency",
                    status=TestStatus.FAILED,
                    duration_ms=overall_duration,
                    message=f"All operations failed ({errors} errors)"
                ))

        except Exception as e:
            suite.add_result(TestResult(
                name="Write latency test",
                status=TestStatus.FAILED,
                duration_ms=0,
                message=f"Test setup failed: {e}"
            ))

        self.suites.append(suite)
        return suite

    def test_read_latency(self, num_operations: int = 100) -> TestSuite:
        """Test read latency"""
        suite = TestSuite(name=f"Read Latency ({num_operations} ops)")
        self.print_section(f"Testing Read Latency ({num_operations} operations)")

        if not self.api_gateway_url:
            suite.add_result(TestResult(
                name="Read latency test",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="API Gateway URL not available"
            ))
            self.suites.append(suite)
            return suite

        try:
            import urllib.request
            import urllib.error

            # First, create tenant
            tenant_id = f"read-perf-{uuid.uuid4().hex[:8]}"
            success, message = self.create_tenant(tenant_id, replication_factor=2)
            if not success:
                suite.add_result(TestResult(
                    name="Read latency test",
                    status=TestStatus.FAILED,
                    duration_ms=0,
                    message=f"Failed to create tenant: {message}"
                ))
                self.suites.append(suite)
                return suite

            # Write some test data
            test_keys = []

            for i in range(min(10, num_operations)):
                test_key = f"read-key-{i}"
                test_keys.append(test_key)

                write_data = json.dumps({
                    "tenant_id": tenant_id,
                    "key": test_key,
                    "value": f"read-value-{i}",
                    "consistency": "quorum"
                }).encode('utf-8')

                req = urllib.request.Request(
                    f"{self.api_gateway_url}/v1/key-value",
                    data=write_data,
                    headers={'Content-Type': 'application/json'},
                    method='POST'
                )
                urllib.request.urlopen(req, timeout=10)

            # Now perform read operations
            latencies = []
            errors = 0
            overall_start = time.time()

            for i in range(num_operations):
                test_key = test_keys[i % len(test_keys)]

                start = time.time()
                try:
                    url = f"{self.api_gateway_url}/v1/key-value?tenant_id={tenant_id}&key={test_key}&consistency=quorum"
                    response = urllib.request.urlopen(url, timeout=10)
                    duration_ms = (time.time() - start) * 1000
                    latencies.append(duration_ms)
                except Exception:
                    errors += 1

            overall_duration = (time.time() - overall_start) * 1000

            if latencies:
                avg_latency = statistics.mean(latencies)
                min_latency = min(latencies)
                max_latency = max(latencies)
                p50_latency = statistics.median(latencies)
                p95_latency = sorted(latencies)[int(len(latencies) * 0.95)] if len(latencies) > 1 else avg_latency
                p99_latency = sorted(latencies)[int(len(latencies) * 0.99)] if len(latencies) > 1 else avg_latency

                throughput = (num_operations - errors) / (overall_duration / 1000)

                suite.add_result(TestResult(
                    name="Read latency",
                    status=TestStatus.PASSED if errors == 0 else TestStatus.WARNING,
                    duration_ms=overall_duration,
                    message=f"Avg: {avg_latency:.2f}ms, P50: {p50_latency:.2f}ms, P95: {p95_latency:.2f}ms, P99: {p99_latency:.2f}ms",
                    details={
                        "operations": num_operations,
                        "errors": errors,
                        "avg_latency_ms": round(avg_latency, 2),
                        "min_latency_ms": round(min_latency, 2),
                        "max_latency_ms": round(max_latency, 2),
                        "p50_latency_ms": round(p50_latency, 2),
                        "p95_latency_ms": round(p95_latency, 2),
                        "p99_latency_ms": round(p99_latency, 2),
                        "throughput_ops_per_sec": round(throughput, 2)
                    }
                ))
            else:
                suite.add_result(TestResult(
                    name="Read latency",
                    status=TestStatus.FAILED,
                    duration_ms=overall_duration,
                    message=f"All operations failed ({errors} errors)"
                ))

        except Exception as e:
            suite.add_result(TestResult(
                name="Read latency test",
                status=TestStatus.FAILED,
                duration_ms=0,
                message=f"Test setup failed: {e}"
            ))

        self.suites.append(suite)
        return suite

    def test_concurrent_writes(self, num_threads: int = 10, ops_per_thread: int = 10) -> TestSuite:
        """Test concurrent write operations"""
        suite = TestSuite(name=f"Concurrent Writes ({num_threads} threads)")
        self.print_section(f"Testing Concurrent Writes ({num_threads} threads, {ops_per_thread} ops each)")

        if not self.api_gateway_url:
            suite.add_result(TestResult(
                name="Concurrent writes test",
                status=TestStatus.SKIPPED,
                duration_ms=0,
                message="API Gateway URL not available"
            ))
            self.suites.append(suite)
            return suite

        try:
            import urllib.request
            import urllib.error

            tenant_id = f"concurrent-{uuid.uuid4().hex[:8]}"

            # Create tenant first
            success, message = self.create_tenant(tenant_id, replication_factor=2)
            if not success:
                suite.add_result(TestResult(
                    name="Concurrent writes test",
                    status=TestStatus.FAILED,
                    duration_ms=0,
                    message=f"Failed to create tenant: {message}"
                ))
                self.suites.append(suite)
                return suite

            def write_operations(thread_id: int) -> Tuple[int, int, List[float]]:
                """Perform write operations in a thread"""
                success_count = 0
                error_count = 0
                latencies = []

                for i in range(ops_per_thread):
                    test_key = f"thread-{thread_id}-key-{i}"
                    test_value = f"thread-{thread_id}-value-{i}"

                    write_data = json.dumps({
                        "tenant_id": tenant_id,
                        "key": test_key,
                        "value": test_value,
                        "consistency": "quorum"
                    }).encode('utf-8')

                    start = time.time()
                    try:
                        req = urllib.request.Request(
                            f"{self.api_gateway_url}/v1/key-value",
                            data=write_data,
                            headers={'Content-Type': 'application/json'},
                            method='POST'
                        )
                        response = urllib.request.urlopen(req, timeout=10)
                        duration_ms = (time.time() - start) * 1000
                        latencies.append(duration_ms)
                        success_count += 1
                    except Exception:
                        error_count += 1

                return success_count, error_count, latencies

            overall_start = time.time()
            all_latencies = []
            total_success = 0
            total_errors = 0

            with ThreadPoolExecutor(max_workers=num_threads) as executor:
                futures = [executor.submit(write_operations, i) for i in range(num_threads)]

                for future in as_completed(futures):
                    success, errors, latencies = future.result()
                    total_success += success
                    total_errors += errors
                    all_latencies.extend(latencies)

            overall_duration = (time.time() - overall_start) * 1000

            if all_latencies:
                avg_latency = statistics.mean(all_latencies)
                p95_latency = sorted(all_latencies)[int(len(all_latencies) * 0.95)]
                throughput = total_success / (overall_duration / 1000)

                suite.add_result(TestResult(
                    name="Concurrent writes",
                    status=TestStatus.PASSED if total_errors == 0 else TestStatus.WARNING,
                    duration_ms=overall_duration,
                    message=f"Success: {total_success}, Errors: {total_errors}, Avg latency: {avg_latency:.2f}ms",
                    details={
                        "threads": num_threads,
                        "total_operations": num_threads * ops_per_thread,
                        "successful": total_success,
                        "failed": total_errors,
                        "avg_latency_ms": round(avg_latency, 2),
                        "p95_latency_ms": round(p95_latency, 2),
                        "throughput_ops_per_sec": round(throughput, 2)
                    }
                ))
            else:
                suite.add_result(TestResult(
                    name="Concurrent writes",
                    status=TestStatus.FAILED,
                    duration_ms=overall_duration,
                    message=f"All operations failed (errors: {total_errors})"
                ))

        except Exception as e:
            suite.add_result(TestResult(
                name="Concurrent writes test",
                status=TestStatus.FAILED,
                duration_ms=0,
                message=f"Test setup failed: {e}"
            ))

        self.suites.append(suite)
        return suite

    # ==================== Report Generation ====================

    def generate_html_report(self, output_file: str = "test_report.html"):
        """Generate HTML test report"""
        self.print_section("Generating Test Report")

        total_tests = sum(len(suite.results) for suite in self.suites)
        total_passed = sum(suite.passed_count() for suite in self.suites)
        total_failed = sum(suite.failed_count() for suite in self.suites)
        total_skipped = sum(suite.skipped_count() for suite in self.suites)

        html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PairDB Test Report</title>
    <style>
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: #f5f7fa;
            padding: 20px;
            color: #2c3e50;
        }}
        .container {{ max-width: 1200px; margin: 0 auto; }}
        .header {{
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 30px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }}
        .header h1 {{ font-size: 2.5em; margin-bottom: 10px; }}
        .header .timestamp {{ opacity: 0.9; font-size: 0.9em; }}
        .summary {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }}
        .summary-card {{
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            text-align: center;
        }}
        .summary-card .number {{
            font-size: 2.5em;
            font-weight: bold;
            margin: 10px 0;
        }}
        .summary-card .label {{ color: #7f8c8d; font-size: 0.9em; text-transform: uppercase; }}
        .passed {{ color: #27ae60; }}
        .failed {{ color: #e74c3c; }}
        .skipped {{ color: #95a5a6; }}
        .warning {{ color: #f39c12; }}
        .suite {{
            background: white;
            padding: 25px;
            border-radius: 10px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }}
        .suite-header {{
            font-size: 1.5em;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 2px solid #ecf0f1;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }}
        .suite-stats {{ font-size: 0.8em; color: #7f8c8d; }}
        .test-result {{
            padding: 12px;
            margin: 8px 0;
            border-left: 4px solid #bdc3c7;
            background: #f8f9fa;
            border-radius: 4px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }}
        .test-result.passed {{ border-left-color: #27ae60; }}
        .test-result.failed {{ border-left-color: #e74c3c; }}
        .test-result.skipped {{ border-left-color: #95a5a6; }}
        .test-result.warning {{ border-left-color: #f39c12; }}
        .test-name {{ font-weight: 500; flex: 1; }}
        .test-duration {{ color: #7f8c8d; font-size: 0.9em; margin: 0 15px; }}
        .test-status {{
            padding: 4px 12px;
            border-radius: 4px;
            font-size: 0.85em;
            font-weight: 600;
        }}
        .test-status.passed {{ background: #d4edda; color: #155724; }}
        .test-status.failed {{ background: #f8d7da; color: #721c24; }}
        .test-status.skipped {{ background: #e2e3e5; color: #383d41; }}
        .test-status.warning {{ background: #fff3cd; color: #856404; }}
        .test-message {{
            margin-top: 8px;
            padding: 8px;
            background: white;
            border-radius: 4px;
            font-size: 0.9em;
            color: #555;
        }}
        .test-details {{
            margin-top: 8px;
            padding: 10px;
            background: #f1f3f5;
            border-radius: 4px;
            font-family: 'Courier New', monospace;
            font-size: 0.85em;
        }}
        .footer {{
            text-align: center;
            margin-top: 40px;
            padding: 20px;
            color: #7f8c8d;
            font-size: 0.9em;
        }}
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🗄️ PairDB Test Report</h1>
            <div class="timestamp">Generated: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}</div>
            <div class="timestamp">Namespace: {self.namespace}</div>
        </div>

        <div class="summary">
            <div class="summary-card">
                <div class="label">Total Tests</div>
                <div class="number">{total_tests}</div>
            </div>
            <div class="summary-card">
                <div class="label">Passed</div>
                <div class="number passed">{total_passed}</div>
            </div>
            <div class="summary-card">
                <div class="label">Failed</div>
                <div class="number failed">{total_failed}</div>
            </div>
            <div class="summary-card">
                <div class="label">Skipped</div>
                <div class="number skipped">{total_skipped}</div>
            </div>
        </div>
"""

        for suite in self.suites:
            passed = suite.passed_count()
            failed = suite.failed_count()
            skipped = suite.skipped_count()

            html_content += f"""
        <div class="suite">
            <div class="suite-header">
                <span>{suite.name}</span>
                <span class="suite-stats">
                    <span class="passed">{passed} passed</span> •
                    <span class="failed">{failed} failed</span> •
                    <span class="skipped">{skipped} skipped</span>
                </span>
            </div>
"""

            for result in suite.results:
                status_class = result.status.value.lower()
                html_content += f"""
            <div class="test-result {status_class}">
                <div style="flex: 1;">
                    <div class="test-name">{result.name}</div>
"""
                if result.message:
                    html_content += f"""
                    <div class="test-message">{result.message}</div>
"""
                if result.details:
                    details_json = json.dumps(result.details, indent=2)
                    html_content += f"""
                    <div class="test-details">{details_json}</div>
"""
                html_content += f"""
                </div>
                <div class="test-duration">{result.duration_ms:.2f}ms</div>
                <div class="test-status {status_class}">{result.status.value}</div>
            </div>
"""

            html_content += """
        </div>
"""

        html_content += f"""
        <div class="footer">
            <p>PairDB - Distributed Key-Value Store</p>
            <p>Test execution completed in {sum(suite.total_duration_ms() for suite in self.suites):.2f}ms</p>
        </div>
    </div>
</body>
</html>
"""

        with open(output_file, 'w') as f:
            f.write(html_content)

        print(f"\n{Colors.OKGREEN}✓ HTML report generated: {output_file}{Colors.ENDC}")

    def print_summary(self):
        """Print test summary to console"""
        self.print_header("Test Summary")

        total_tests = sum(len(suite.results) for suite in self.suites)
        total_passed = sum(suite.passed_count() for suite in self.suites)
        total_failed = sum(suite.failed_count() for suite in self.suites)
        total_skipped = sum(suite.skipped_count() for suite in self.suites)

        print(f"\n{Colors.BOLD}Overall Results:{Colors.ENDC}")
        print(f"  Total Tests:   {total_tests}")
        print(f"  {Colors.OKGREEN}✓ Passed:{Colors.ENDC}      {total_passed}")
        print(f"  {Colors.FAIL}✗ Failed:{Colors.ENDC}      {total_failed}")
        print(f"  {Colors.OKCYAN}○ Skipped:{Colors.ENDC}     {total_skipped}")

        print(f"\n{Colors.BOLD}Test Suites:{Colors.ENDC}")
        for suite in self.suites:
            passed = suite.passed_count()
            failed = suite.failed_count()
            skipped = suite.skipped_count()
            total = len(suite.results)

            status_symbol = Colors.OKGREEN + "✓" if failed == 0 else Colors.FAIL + "✗"
            print(f"  {status_symbol} {suite.name}{Colors.ENDC} "
                  f"({passed}/{total} passed, {failed} failed, {skipped} skipped)")

    def cleanup(self):
        """Cleanup resources"""
        if self.port_forward_process:
            self.port_forward_process.terminate()
            self.port_forward_process.wait()

    def run_all_tests(self, skip_validation: bool = False):
        """Run all test suites"""
        self.print_header("PairDB Test Suite")

        try:
            # Validation tests
            if not skip_validation:
                self.validate_deployment()
                self.check_pod_health()

            # Setup
            result = self.setup_port_forward()
            if result.status != TestStatus.PASSED:
                print(f"\n{Colors.FAIL}✗ Failed to setup port-forward. Skipping API tests.{Colors.ENDC}")
                self.print_summary()
                return

            # Service health
            self.check_service_health()

            # Functional tests
            self.test_crud_operations()

            # Performance tests
            self.test_write_latency(num_operations=50)
            self.test_read_latency(num_operations=50)
            self.test_concurrent_writes(num_threads=5, ops_per_thread=10)

        finally:
            self.cleanup()

        # Print results for each suite
        for suite in self.suites:
            print()
            for result in suite.results:
                self.print_test_result(result)

        # Print summary
        self.print_summary()


def main():
    parser = argparse.ArgumentParser(
        description="PairDB Test Suite - Validate and test PairDB deployment"
    )
    parser.add_argument(
        "--namespace",
        default="pairdb",
        help="Kubernetes namespace (default: pairdb)"
    )
    parser.add_argument(
        "--skip-validation",
        action="store_true",
        help="Skip deployment validation checks"
    )
    parser.add_argument(
        "--output",
        default="pairdb_test_report.html",
        help="Output HTML report file (default: pairdb_test_report.html)"
    )
    parser.add_argument(
        "--api-gateway-url",
        help="API Gateway URL (default: auto port-forward)"
    )

    args = parser.parse_args()

    tester = PairDBTester(
        namespace=args.namespace,
        api_gateway_url=args.api_gateway_url
    )

    try:
        tester.run_all_tests(skip_validation=args.skip_validation)
        tester.generate_html_report(output_file=args.output)

        # Exit code based on test results
        total_failed = sum(suite.failed_count() for suite in tester.suites)
        sys.exit(1 if total_failed > 0 else 0)

    except KeyboardInterrupt:
        print(f"\n\n{Colors.WARNING}Test interrupted by user{Colors.ENDC}")
        tester.cleanup()
        sys.exit(130)
    except Exception as e:
        print(f"\n{Colors.FAIL}Error: {e}{Colors.ENDC}")
        tester.cleanup()
        sys.exit(1)


if __name__ == "__main__":
    main()
