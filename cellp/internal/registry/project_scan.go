package registry

import (
	"database/sql"
	"time"
)

const projectSelectCols = `id, git_remote, prod_version_id, previous_prod_version_id, previous_prod_at, ingress_tier_b, prod_listen_port, created_at`

const projectListSelectCols = `p.id, p.git_remote, p.prod_version_id, p.previous_prod_version_id, p.previous_prod_at, p.ingress_tier_b, p.prod_listen_port, p.created_at`

func scanProject(row scanner) (*Project, error) {
	var p Project
	var gitRemote, prod, prevProd, prevProdAt, tierB sql.NullString
	var prodPort sql.NullInt64
	var created string
	if err := row.Scan(&p.ID, &gitRemote, &prod, &prevProd, &prevProdAt, &tierB, &prodPort, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if gitRemote.Valid {
		p.GitRemote = &gitRemote.String
	}
	if prod.Valid {
		p.ProdVersionID = &prod.String
	}
	if prevProd.Valid {
		p.PreviousProdVersionID = &prevProd.String
	}
	if prevProdAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, prevProdAt.String)
		p.PreviousProdAt = &t
	}
	if tierB.Valid {
		v := tierB.String
		p.IngressTierB = &v
	}
	if prodPort.Valid {
		v := int(prodPort.Int64)
		p.ProdListenPort = &v
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &p, nil
}
