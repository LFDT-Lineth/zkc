;; As permutation_22, but without a target selector.
(defcolumns (X :i16) (P :binary) (Y :i16))
(defpermutation test ((P X)) ((_ Y)))
