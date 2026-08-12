CREATE TABLE `post_publish_log` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `post_id` BIGINT UNSIGNED NOT NULL,
  `published_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_post_publish_log_post_id_published_at` (`post_id`, `published_at`),
  CONSTRAINT `fk_post_publish_log_post_id` FOREIGN KEY (`post_id`) REFERENCES `posts` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
