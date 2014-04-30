package models

func Fix() error {
	_, err := orm.Exec("alter table repository drop column num_releases")
	return err
}
