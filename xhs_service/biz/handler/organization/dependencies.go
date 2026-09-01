package organization

import service "media_agent/xhs_service/biz/service/organization"

var organizationService *service.Service

func SetService(value *service.Service) { organizationService = value }
