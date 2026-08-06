(defcolumns (sel :i1) (from :i16) (into :i8) (tmp :i8))
;; tmp holds the (narrowed) value of from on selected rows
(defconstraint tmp_def () (if (== sel 1) (== tmp from)))

(defclookup
  l1
  ;; target column
  (into)
  ;; source selector
  sel
  ;; source column
  (tmp)
)
