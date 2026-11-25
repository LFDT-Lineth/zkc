;; Permutation whose target column holds the previous row of another column.
(defcolumns (X :i16) (Y :i16) (PY :i16) (one :i1 :constant 1))
;; PY holds the value of the expression (shift Y -1).
(defconstraint py_def () (== PY (shift Y -1)))
(defpermutation test ((one X)) ((one PY)))
