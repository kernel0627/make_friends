package scenarios

// AllScenarios returns all hand-written scenarios for seed generation and evaluation.
func AllScenarios() []*Scenario {
	return []*Scenario{
		// Content reports
		ContentCommercialReport(),
		ContentOffPlatformLegitimate(),
		ContentReportFalsePositive(),
		ContentRepeatReporter(),

		// Settlement disputes
		SettlementNoShowClear(),
		SettlementMaterialChange(),
		SettlementDisputeAmbiguous(),
		SettlementMultipartyDispute(),
		SettlementBoundaryCancelTiming(),

		// Moderation appeals
		ModerationAppealLegitimate(),
		ModerationAppealGuilty(),
		ModerationAppealEscalation(),

		// Credit appeals
		CreditAppealAfterNoShow(),
		CreditAppealUnfounded(),
		CreditContradictoryEvidence(),
	}
}
