import unittest

from healthcare_actuator import OperationRequest, evaluate_operation


class HealthcareActuatorTest(unittest.TestCase):
    def test_gold_card_threshold_transitions_to_auto_approved(self):
        request = OperationRequest(
            action="gold-card",
            request_id="PA-2026-0043",
            resource_type="ClaimResponse",
            subject="Dr. Priya Nair",
            measured_value=96,
            threshold_value=90,
            run_id="healthcare-run-123",
            scenario_id="healthcare-gold-card",
        )

        observation = evaluate_operation(request, "2026-09-01T12:30:00Z")

        self.assertEqual("AUTO_APPROVED", observation.status)
        self.assertTrue(observation.auto_approved)
        self.assertFalse(observation.reportable_to_oha)
        self.assertEqual(request.run_id, observation.run_id)
        self.assertEqual(request.scenario_id, observation.scenario_id)

    def test_gold_card_below_threshold_remains_pending(self):
        request = OperationRequest(
            action="gold-card",
            request_id="PA-2026-0043",
            resource_type="ClaimResponse",
            subject="Dr. Priya Nair",
            measured_value=89,
            threshold_value=90,
            run_id="healthcare-run-123",
            scenario_id="healthcare-gold-card",
        )

        observation = evaluate_operation(request, "2026-09-01T12:30:00Z")

        self.assertEqual("PENDING_REVIEW", observation.status)
        self.assertFalse(observation.auto_approved)

    def test_sla_elapsed_days_transitions_to_reportable_breach(self):
        request = OperationRequest(
            action="sla-check",
            request_id="PA-2026-0044",
            resource_type="ClaimResponse",
            subject="Dr. James O'Brien",
            measured_value=10,
            threshold_value=7,
            run_id="healthcare-run-456",
            scenario_id="healthcare-sla-breach",
        )

        observation = evaluate_operation(request, "2026-09-01T12:30:00Z")

        self.assertEqual("SLA_BREACHED", observation.status)
        self.assertFalse(observation.auto_approved)
        self.assertTrue(observation.reportable_to_oha)
        self.assertEqual(request.run_id, observation.run_id)
        self.assertEqual(request.scenario_id, observation.scenario_id)

    def test_sla_within_threshold_remains_in_review(self):
        request = OperationRequest(
            action="sla-check",
            request_id="PA-2026-0044",
            resource_type="ClaimResponse",
            subject="Dr. James O'Brien",
            measured_value=7,
            threshold_value=7,
            run_id="healthcare-run-456",
            scenario_id="healthcare-sla-breach",
        )

        observation = evaluate_operation(request, "2026-09-01T12:30:00Z")

        self.assertEqual("IN_REVIEW", observation.status)
        self.assertFalse(observation.reportable_to_oha)

    def test_unknown_action_fails_closed(self):
        request = OperationRequest(
            action="unknown",
            request_id="PA-2026-0044",
            resource_type="ClaimResponse",
            subject="Dr. James O'Brien",
            measured_value=10,
            threshold_value=7,
            run_id="healthcare-run-456",
            scenario_id="healthcare-sla-breach",
        )

        with self.assertRaises(ValueError):
            evaluate_operation(request, "2026-09-01T12:30:00Z")


if __name__ == "__main__":
    unittest.main()
