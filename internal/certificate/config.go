package certificate

import "github.com/jerkytreats/dns/internal/config"

// Configuration keys for certificate management
const (
	CertEmailKey                = "certificate.email"
	CertDomainKey               = "certificate.domain"
	CertCertFileKey             = "server.tls.cert_file"
	CertKeyFileKey              = "server.tls.key_file"
	CertCADirURLKey             = "certificate.ca_dir_url"
	CertInsecureSkipVerifyKey   = "certificate.insecure_skip_verify"
	CertRenewalEnabledKey       = "certificate.renewal.enabled"
	CertRenewalRenewBeforeKey   = "certificate.renewal.renew_before"
	CertRenewalCheckIntervalKey = "certificate.renewal.check_interval"
	CertDNSResolversKey         = "certificate.dns_resolvers"
	CertDNSTimeoutKey           = "certificate.dns_timeout"
	CertCloudflareTokenKey      = "certificate.cloudflare_api_token"
	CertDNSCleanupWaitKey       = "certificate.dns_cleanup_wait"
	CertDNSCreationWaitKey      = "certificate.dns_creation_wait"
	CertUseProdCertsKey         = "certificate.use_production_certs"
	CertDomainStoragePathKey    = "certificate.domain_storage_path"
	CertDomainBackupCountKey    = "certificate.domain_backup_count"
)

func init() {
	config.RegisterRequiredKey(CertEmailKey)
	config.RegisterRequiredKey(CertDomainKey)
	config.RegisterRequiredKey(CertCertFileKey)
	config.RegisterRequiredKey(CertKeyFileKey)
	config.RegisterRequiredKey(CertCADirURLKey)
	config.RegisterRequiredKey(CertRenewalEnabledKey)
	config.RegisterRequiredKey(CertRenewalRenewBeforeKey)
	config.RegisterRequiredKey(CertRenewalCheckIntervalKey)
	config.RegisterRequiredKey(CertDNSResolversKey)
	config.RegisterRequiredKey(CertDNSTimeoutKey)
	config.RegisterRequiredKey(CertCloudflareTokenKey)
	config.RegisterRequiredKey(CertDNSCleanupWaitKey)
	config.RegisterRequiredKey(CertDNSCreationWaitKey)
	config.RegisterRequiredKey(CertUseProdCertsKey)
}
