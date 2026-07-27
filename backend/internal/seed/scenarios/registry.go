package scenarios

// AllScenarios returns all hand-written scenarios for seed generation and evaluation.
func AllScenarios() []*Scenario {
	return []*Scenario{
		// Content reports
		ContentCommercialReport(),
		ContentOffPlatformLegitimate(),
		ContentReportFalsePositive(),

		// Settlement disputes
		SettlementNoShowClear(),
		SettlementMaterialChange(),
		SettlementDisputeAmbiguous(),

		// Moderation appeals
		ModerationAppealLegitimate(),
		ModerationAppealGuilty(),

		// Credit appeals
		CreditAppealAfterNoShow(),
		CreditAppealUnfounded(),
	}
}
