package crawl

import service "media_agent/xhs_service/biz/service/crawl"

var crawlService *service.Service

func SetService(value *service.Service) { crawlService = value }
