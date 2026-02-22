package nasr

type foreignKey struct {
	childTable  string
	columns     []string
	parentTable string
}

func foreignKeyDefs() []foreignKey {
	return []foreignKey{
		{"APT_RWY", []string{"SITE_NO"}, "APT_BASE"},
		{"APT_RWY_END", []string{"SITE_NO", "RWY_ID"}, "APT_RWY"},
		{"APT_ARS", []string{"SITE_NO", "RWY_ID", "RWY_END_ID"}, "APT_RWY_END"},
		{"APT_ATT", []string{"SITE_NO"}, "APT_BASE"},
		{"APT_CON", []string{"SITE_NO"}, "APT_BASE"},
		{"APT_RMK", []string{"SITE_NO"}, "APT_BASE"},
		{"NAV_CKPT", []string{"NAV_ID", "NAV_TYPE"}, "NAV_BASE"},
		{"NAV_RMK", []string{"NAV_ID", "NAV_TYPE"}, "NAV_BASE"},
		{"FIX_CHRT", []string{"FIX_ID", "ICAO_REGION_CODE"}, "FIX_BASE"},
		{"FIX_NAV", []string{"FIX_ID", "ICAO_REGION_CODE"}, "FIX_BASE"},
		{"AWY_SEG_ALT", []string{"AWY_ID"}, "AWY_BASE"},
		{"ILS_GS", []string{"SITE_NO", "RWY_END_ID", "ILS_LOC_ID"}, "ILS_BASE"},
		{"ILS_DME", []string{"SITE_NO", "RWY_END_ID", "ILS_LOC_ID"}, "ILS_BASE"},
		{"ILS_MKR", []string{"SITE_NO", "RWY_END_ID", "ILS_LOC_ID"}, "ILS_BASE"},
		{"ILS_RMK", []string{"SITE_NO", "RWY_END_ID", "ILS_LOC_ID"}, "ILS_BASE"},
		{"ATC_SVC", []string{"FACILITY_ID", "FACILITY_TYPE"}, "ATC_BASE"},
		{"ATC_ATIS", []string{"FACILITY_ID", "FACILITY_TYPE"}, "ATC_BASE"},
		{"ATC_RMK", []string{"FACILITY_ID", "FACILITY_TYPE"}, "ATC_BASE"},
		{"STAR_APT", []string{"STAR_COMPUTER_CODE"}, "STAR_BASE"},
		{"STAR_RTE", []string{"STAR_COMPUTER_CODE"}, "STAR_BASE"},
		{"DP_APT", []string{"DP_COMPUTER_CODE"}, "DP_BASE"},
		{"DP_RTE", []string{"DP_COMPUTER_CODE"}, "DP_BASE"},
		{"HPF_SPD_ALT", []string{"HP_NAME", "HP_NO"}, "HPF_BASE"},
		{"HPF_CHRT", []string{"HP_NAME", "HP_NO"}, "HPF_BASE"},
		{"HPF_RMK", []string{"HP_NAME", "HP_NO"}, "HPF_BASE"},
		{"PFR_SEG", []string{"ORIGIN_ID", "DSTN_ID", "PFR_TYPE_CODE", "ROUTE_NO"}, "PFR_BASE"},
		{"MTR_PT", []string{"ROUTE_TYPE_CODE", "ROUTE_ID"}, "MTR_BASE"},
		{"MTR_AGY", []string{"ROUTE_TYPE_CODE", "ROUTE_ID"}, "MTR_BASE"},
		{"MTR_SOP", []string{"ROUTE_TYPE_CODE", "ROUTE_ID"}, "MTR_BASE"},
		{"MTR_TERR", []string{"ROUTE_TYPE_CODE", "ROUTE_ID"}, "MTR_BASE"},
		{"MTR_WDTH", []string{"ROUTE_TYPE_CODE", "ROUTE_ID"}, "MTR_BASE"},
		{"ARB_SEG", []string{"LOCATION_ID"}, "ARB_BASE"},
		{"WXL_SVC", []string{"WEA_ID"}, "WXL_BASE"},
		{"MAA_SHP", []string{"MAA_ID"}, "MAA_BASE"},
		{"MAA_RMK", []string{"MAA_ID"}, "MAA_BASE"},
		{"MAA_CON", []string{"MAA_ID"}, "MAA_BASE"},
		{"PJA_CON", []string{"PJA_ID"}, "PJA_BASE"},
		{"FSS_RMK", []string{"FSS_ID"}, "FSS_BASE"},
	}
}
