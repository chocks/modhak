-- Dev-only read-only user. Grants are proposals; see docs/FEATURES.md section 4
-- (UNVERIFIED except where noted there).
CREATE USER 'modhak'@'%' IDENTIFIED BY 'modhak';
GRANT SELECT, PROCESS ON *.* TO 'modhak'@'%';
