;; Permutation between two columns of the same module, without selectors.
(defcolumns (X :i16) (Y :i16))
(defpermutation test ((_ X)) ((_ Y)))
