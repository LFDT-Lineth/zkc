;; As permutation_27, but without a target selector.
(defcolumns (X :i17) (P :binary) (Y :i16))
(defpermutation (test :unchecked) ((P X)) ((_ Y)))
