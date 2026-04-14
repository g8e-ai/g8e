# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import pytest

from app.dependencies import is_infrastructure_health_check_ip

pytestmark = pytest.mark.unit


def test_google_lb_range_35_191():
    assert is_infrastructure_health_check_ip("35.191.0.1") is True
    assert is_infrastructure_health_check_ip("35.191.255.255") is True


def test_google_lb_range_130_211_within_22():
    assert is_infrastructure_health_check_ip("130.211.0.1") is True
    assert is_infrastructure_health_check_ip("130.211.3.254") is True


def test_google_lb_range_130_211_outside_22():
    assert is_infrastructure_health_check_ip("130.211.4.0") is False
    assert is_infrastructure_health_check_ip("130.211.100.1") is False


def test_internal_docker_network_10_x():
    assert is_infrastructure_health_check_ip("10.0.0.1") is True
    assert is_infrastructure_health_check_ip("10.96.0.52") is True


def test_public_ip_not_health_check():
    assert is_infrastructure_health_check_ip("203.0.113.1") is False
    assert is_infrastructure_health_check_ip("8.8.8.8") is False


def test_empty_string_returns_false():
    assert is_infrastructure_health_check_ip("") is False


def test_ipv6_mapped_ipv4_normalised():
    assert is_infrastructure_health_check_ip("::ffff:10.0.0.1") is True
    assert is_infrastructure_health_check_ip("::ffff:35.191.5.5") is True
