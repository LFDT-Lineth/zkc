(defcolumns (X :i16) (Y :i16) (PY :i16))
;; PY holds the value of the expression (shift Y -1)
(defconstraint py_def () (== PY (shift Y -1)))
(deflookup test (PY) (X))
