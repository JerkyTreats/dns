# Use official CoreDNS image
FROM coredns/coredns:latest

# Copy Corefile and zone definitions
COPY coredns/Corefile /Corefile
COPY coredns/zones/ /zones/

# Default command to run CoreDNS with the Corefile
CMD ["-conf", "/Corefile"]
