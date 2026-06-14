package database

import (
	"fmt"

	"pos-backend/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Jakarta",
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return db, nil
}

func EnsurePaymentMethodConstraint(db *gorm.DB) error {
	return db.Exec(`
DO $$
DECLARE
	constraint_record RECORD;
BEGIN
	FOR constraint_record IN
		SELECT conname
		FROM pg_constraint
		WHERE conrelid = 'payments'::regclass
			AND contype = 'c'
			AND pg_get_constraintdef(oid) ILIKE '%payment_method%'
	LOOP
		EXECUTE format('ALTER TABLE payments DROP CONSTRAINT IF EXISTS %I', constraint_record.conname);
	END LOOP;

	ALTER TABLE payments
		ADD CONSTRAINT chk_payments_payment_method
		CHECK (payment_method IN ('cash','qris'));
END $$;
`).Error
}

func EnsurePaymentStatusConstraint(db *gorm.DB) error {
	return db.Exec(`
DO $$
DECLARE
	constraint_record RECORD;
BEGIN
	FOR constraint_record IN
		SELECT conname
		FROM pg_constraint
		WHERE conrelid = 'payments'::regclass
			AND contype = 'c'
			AND pg_get_constraintdef(oid) ILIKE '%status%'
	LOOP
		EXECUTE format('ALTER TABLE payments DROP CONSTRAINT IF EXISTS %I', constraint_record.conname);
	END LOOP;

	ALTER TABLE payments
		ADD CONSTRAINT chk_payments_status
		CHECK (status IN ('unpaid','waiting_confirmation','pending','paid','rejected','failed','cancelled'));
END $$;
`).Error
}

func EnsureTableStatusIsOptional(db *gorm.DB) error {
	return db.Exec(`
DO $$
DECLARE
	constraint_record RECORD;
BEGIN
	FOR constraint_record IN
		SELECT conname
		FROM pg_constraint
		WHERE conrelid = 'restaurant_tables'::regclass
			AND contype = 'c'
			AND pg_get_constraintdef(oid) ILIKE '%status%'
	LOOP
		EXECUTE format('ALTER TABLE restaurant_tables DROP CONSTRAINT IF EXISTS %I', constraint_record.conname);
	END LOOP;

	ALTER TABLE restaurant_tables ALTER COLUMN status DROP NOT NULL;
	ALTER TABLE restaurant_tables ALTER COLUMN status DROP DEFAULT;
END $$;
`).Error
}
