CREATE TABLE `posts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  -- public_id: UUID v7 (RFC 9562), CHAR(36) canonical 8-4-4-4-12 form.
  -- Chosen over ULID/CHAR(26): standard UUID, time-ordered for indexes/logs, matches docs/architecture.md §8.
  `public_id` CHAR(36) NOT NULL,
  `author_id` BIGINT UNSIGNED NOT NULL,
  `title` VARCHAR(255) NOT NULL,
  `body` TEXT NOT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'draft',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_posts_public_id` (`public_id`),
  KEY `idx_posts_author_id` (`author_id`),
  KEY `idx_posts_status_created_at` (`status`, `created_at`),
  KEY `idx_posts_deleted_at` (`deleted_at`),
  CONSTRAINT `fk_posts_author_id` FOREIGN KEY (`author_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
