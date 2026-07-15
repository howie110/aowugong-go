// Package rbac 提供角色、页面权限和用户角色分配能力。
package rbac

const (
	AdminRoleCode    = "admin"
	InvestorRoleCode = "investor"

	PermissionFinanceOverview        = "page:finance:overview"
	PermissionWeread                 = "page:weread"
	PermissionFinancePositions       = "page:finance:positions"
	PermissionFinanceStockAnalysis   = "page:finance:stock_analysis"
	PermissionFinanceArticleFetch    = "page:finance:article_fetch"
	PermissionFinanceArticleAnalysis = "page:finance:article_analysis"
	PermissionFinanceBacktest        = "page:finance:backtest"
	PermissionFinanceData            = "page:finance:data"
	PermissionFinanceJobs            = "page:finance:jobs"
	PermissionFinanceTrading         = "page:finance:trading"
	PermissionFinanceNotifications   = "page:finance:notifications"
	PermissionMahjong                = "page:mahjong"
	PermissionSubscriptions          = "page:subscriptions"
	PermissionMonitoring             = "page:monitoring"
	PermissionWork                   = "page:work"
	PermissionPermissions            = "page:permissions"
)

// Permission 描述系统内置页面权限。
type Permission struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Role 描述可分配给用户的角色。
type Role struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	IsSystem    bool   `json:"is_system"`
}

// UserRoles 描述权限页面中的用户与角色关系。
type UserRoles struct {
	ID       int64    `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	IsActive bool     `json:"is_active"`
	Roles    []string `json:"roles"`
}

// DefaultPermissions 是由代码维护的全部页面权限基线。
var DefaultPermissions = []Permission{
	{Code: PermissionFinanceOverview, Name: "控制台", Group: "finance", Description: "查看 finance 控制台总览。"},
	{Code: PermissionWeread, Name: "微信读书", Group: "weread", Description: "查看微信读书看板。"},
	{Code: PermissionFinancePositions, Name: "股票仓位导入", Group: "finance", Description: "查看和上传股票仓位导入记录。"},
	{Code: PermissionFinanceStockAnalysis, Name: "股票仓位分析", Group: "finance", Description: "查看股票仓位分析页面。"},
	{Code: PermissionFinanceArticleFetch, Name: "投资文章抓取", Group: "finance", Description: "查看信息源，并抓取、分析投资文章。"},
	{Code: PermissionFinanceArticleAnalysis, Name: "投资文章分析", Group: "finance", Description: "查看投资文章的信号和短期市场统计。"},
	{Code: PermissionFinanceBacktest, Name: "回测", Group: "finance", Description: "查看回测页面。"},
	{Code: PermissionFinanceData, Name: "数据", Group: "finance", Description: "查看行情和基础数据状态。"},
	{Code: PermissionFinanceJobs, Name: "定时任务", Group: "finance", Description: "查看定时任务页面。"},
	{Code: PermissionFinanceTrading, Name: "交易", Group: "finance", Description: "查看交易模块页面。"},
	{Code: PermissionFinanceNotifications, Name: "通知", Group: "finance", Description: "查看通知页面。"},
	{Code: PermissionMahjong, Name: "麻将战绩", Group: "content", Description: "查看和录入麻将战绩页面。"},
	{Code: PermissionSubscriptions, Name: "订阅管理", Group: "content", Description: "查看和维护订阅服务、费用和到期日。"},
	{Code: PermissionMonitoring, Name: "监控管理", Group: "system", Description: "查看服务连通性监控和最近检测结果。"},
	{Code: PermissionWork, Name: "工作导航", Group: "work", Description: "查看常用系统、工具和资料导航。"},
	{Code: PermissionPermissions, Name: "权限管理", Group: "system", Description: "查看权限管理页面，并给用户加入角色。"},
}

// DefaultRoles 是由代码维护的系统角色基线。
var DefaultRoles = []Role{
	{Code: AdminRoleCode, Name: "管理员", Description: "系统管理员，天然拥有所有权限。", IsActive: true, IsSystem: true},
	{Code: InvestorRoleCode, Name: "投资者", Description: "普通投资者，只能查看已开通的业务页面。", IsActive: true, IsSystem: true},
}
