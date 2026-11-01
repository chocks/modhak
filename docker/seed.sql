-- Small sample schema so later milestones have something to look at.
CREATE DATABASE IF NOT EXISTS shop;
USE shop;
CREATE TABLE customers (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  email VARCHAR(255) NOT NULL,
  created_at DATETIME NOT NULL,
  KEY idx_email (email),
  KEY idx_email_dup (email)
);
CREATE TABLE orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  customer_id INT UNSIGNED NOT NULL,
  status TINYINT NOT NULL,
  KEY idx_customer (customer_id),
  KEY idx_customer_status (customer_id, status)
);
